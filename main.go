package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/burnettdev/adsb2loki/pkg/flightdata"
	"github.com/burnettdev/adsb2loki/pkg/logging"
	"github.com/burnettdev/adsb2loki/pkg/otel/logs"
	"github.com/burnettdev/adsb2loki/pkg/tracing"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file before initializing logger so LOG_LEVEL is available
	envErr := godotenv.Load()

	logging.Init()
	logger := logging.Get()

	logger.DebugCall("main")

	if envErr != nil {
		logger.Debug("Environment file not found (this is normal in production)", "error", envErr)
	} else {
		logger.Debug("Environment file loaded successfully")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize OpenTelemetry tracing
	shutdownTracing, err := tracing.InitTracing()
	if err != nil {
		logger.Error("Failed to initialize OpenTelemetry tracing", "error", err)
		// Continue without tracing rather than failing
	}
	defer shutdownTracing()

	// Initialize OpenTelemetry logging
	shutdownLogs, err := logs.InitLogs()
	if err != nil {
		logger.Error("Failed to initialize OpenTelemetry logging", "error", err)
		// Continue without logging rather than failing
	}
	defer shutdownLogs()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	logger.Info("Starting data fetch loop", "interval", "5s")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Application started successfully")
	for {
		select {
		case <-ticker.C:
			logging.DebugCtx(ctx, "Ticker fired - fetching data")

			if err := flightdata.FetchAndPushLogs(ctx); err != nil {
				logging.ErrorCtx(ctx, "Error fetching and pushing data", "error", err)
			} else {
				logging.DebugCtx(ctx, "Data fetch and push completed successfully")
			}

		case sig := <-sigChan:
			logger.Info("Received shutdown signal", "signal", sig)
			logger.Debug("Graceful shutdown initiated")
			return
		case <-ctx.Done():
			logger.Debug("Context cancelled")
			return
		}
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	logging.DebugCall("getEnvOrDefault", "key", key, "default", defaultValue)

	if value, exists := os.LookupEnv(key); exists {
		logging.Debug("Environment variable found", "key", key, "value", value)
		return value
	}

	logging.Debug("Environment variable not found, using default", "key", key, "default", defaultValue)
	return defaultValue
}
