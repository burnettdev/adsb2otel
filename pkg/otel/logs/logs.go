package logs

import (
	"context"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var (
	globalLoggerProvider *sdklog.LoggerProvider
	mu                   sync.RWMutex
)

// InitLogs initializes OpenTelemetry logging with support for both gRPC and HTTP protocols
func InitLogs() (func(), error) {
	// Check if logging is enabled
	if enabled := getEnv("OTEL_LOGS_ENABLED", "true"); !isTrue(enabled) {
		log.Println("OpenTelemetry logging is disabled")
		return func() {}, nil
	}

	// Get OTLP endpoint from environment variables (shared first, then signal-specific)
	endpoint := getOTLPEndpoint()

	// Check if connection should be insecure (shared first, then signal-specific)
	insecure := getEnvBool("OTEL_EXPORTER_OTLP_INSECURE", "OTEL_EXPORTER_OTLP_LOGS_INSECURE", true)

	// Parse headers if provided (shared first, then signal-specific)
	headers := parseHeaders(getEnvWithFallback("OTEL_EXPORTER_OTLP_HEADERS", "OTEL_EXPORTER_OTLP_LOGS_HEADERS", ""))

	// Determine protocol (http or grpc) - shared first, then signal-specific
	protocol := strings.ToLower(getEnvWithFallback("OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_LOGS_PROTOCOL", "http"))
	if protocol != "http" && protocol != "grpc" {
		log.Printf("Invalid protocol %s, defaulting to http", protocol)
		protocol = "http"
	}

	var exporter sdklog.Exporter
	var err error

	// Create exporter based on protocol
	if protocol == "grpc" {
		opts := []otlploggrpc.Option{
			otlploggrpc.WithEndpoint(endpoint),
		}

		if insecure {
			opts = append(opts, otlploggrpc.WithInsecure())
		}

		if len(headers) > 0 {
			opts = append(opts, otlploggrpc.WithHeaders(headers))
		}

		exporter, err = otlploggrpc.New(context.Background(), opts...)
	} else {
		opts := []otlploghttp.Option{
			otlploghttp.WithEndpoint(endpoint),
		}

		if insecure {
			opts = append(opts, otlploghttp.WithInsecure())
		}

		if len(headers) > 0 {
			opts = append(opts, otlploghttp.WithHeaders(headers))
		}

		exporter, err = otlploghttp.New(context.Background(), opts...)
	}

	if err != nil {
		log.Printf("Failed to create OTLP log exporter, using noop: %v", err)
		// Return a noop shutdown function if exporter creation fails
		return func() {}, nil
	}

	// Create resource with Go-specific attributes
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			// Service identification
			semconv.ServiceName("adsb2otel"),
			semconv.ServiceVersion("1.0.0"),

			// Process and runtime information
			semconv.ProcessRuntimeName("go"),
			semconv.ProcessRuntimeVersion(runtime.Version()),
			semconv.ProcessRuntimeDescription("Go runtime"),
			semconv.ProcessPID(os.Getpid()),

			// Telemetry SDK information
			semconv.TelemetrySDKName("opentelemetry"),
			semconv.TelemetrySDKLanguageGo,
			semconv.TelemetrySDKVersion("1.31.0"),
		),
	)
	if err != nil {
		return nil, err
	}

	// Create batch processor
	processor := sdklog.NewBatchProcessor(exporter)

	// Create logger provider
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(processor),
		sdklog.WithResource(res),
	)

	// Store logger provider globally
	mu.Lock()
	globalLoggerProvider = lp
	mu.Unlock()

	log.Printf("OpenTelemetry logging initialized successfully (protocol: %s, endpoint: %s)", protocol, endpoint)

	return func() {
		if err := lp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down logger provider: %v", err)
		}
	}, nil
}

// GetLoggerProvider returns the global logger provider
func GetLoggerProvider() *sdklog.LoggerProvider {
	mu.RLock()
	defer mu.RUnlock()
	return globalLoggerProvider
}

// GetLogger returns a logger instance for the given name
func GetLogger(name string) otellog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if globalLoggerProvider == nil {
		// Return nil logger if not initialized - caller should check
		return nil
	}
	return globalLoggerProvider.Logger(name)
}

// getEnv returns the value of an environment variable or a default value if not set
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// isTrue checks if a string represents a true value
func isTrue(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes" || s == "on"
}

// getOTLPEndpoint determines the OTLP endpoint from environment variables
// Uses shared OTEL_EXPORTER_OTLP_ENDPOINT first, then signal-specific override
func getOTLPEndpoint() string {
	// Check for shared OTLP endpoint first
	if endpoint := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""); endpoint != "" {
		return cleanEndpoint(endpoint)
	}

	// Fall back to logs-specific endpoint if shared is not set
	if endpoint := getEnv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", ""); endpoint != "" {
		return cleanEndpoint(endpoint)
	}

	// Default to localhost (HTTP default port)
	return "localhost:4318"
}

// cleanEndpoint removes protocol and path from endpoint URL
func cleanEndpoint(endpoint string) string {
	// Remove http:// or https:// prefix if present
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	// Remove /v1/logs suffix if present since WithEndpoint handles the path separately
	if strings.HasSuffix(endpoint, "/v1/logs") {
		endpoint = strings.TrimSuffix(endpoint, "/v1/logs")
	}

	// Remove any trailing slashes
	endpoint = strings.TrimSuffix(endpoint, "/")

	return endpoint
}

// getEnvWithFallback returns the value of the primary env var, or falls back to secondary
func getEnvWithFallback(primary, secondary, defaultValue string) string {
	if value := getEnv(primary, ""); value != "" {
		return value
	}
	return getEnv(secondary, defaultValue)
}

// getEnvBool returns a boolean value, checking primary env var first, then secondary
func getEnvBool(primary, secondary string, defaultValue bool) bool {
	if value := getEnv(primary, ""); value != "" {
		return isTrue(value)
	}
	if value := getEnv(secondary, ""); value != "" {
		return isTrue(value)
	}
	return defaultValue
}

// parseHeaders parses header string in format "key1=value1,key2=value2"
func parseHeaders(headerStr string) map[string]string {
	headers := make(map[string]string)
	if headerStr == "" {
		return headers
	}

	pairs := strings.Split(headerStr, ",")
	for _, pair := range pairs {
		if kv := strings.SplitN(strings.TrimSpace(pair), "=", 2); len(kv) == 2 {
			headers[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	return headers
}
