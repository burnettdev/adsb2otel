package flightdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/burnettdev/adsb2loki/pkg/logging"
	"github.com/burnettdev/adsb2loki/pkg/models"
	"github.com/burnettdev/adsb2loki/pkg/otel/logs"
)

var (
	httpClient = &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   30 * time.Second,
	}
	tracer = otel.Tracer("flightdata-client")
)

func FetchAndPushLogs(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "flightdata.fetch_and_push",
		trace.WithAttributes(
			attribute.String("service", "adsb"),
		),
	)
	defer span.End()

	logging.DebugCallCtx(ctx, "FetchAndPushLogs")

	flightDataURL := os.Getenv("FLIGHT_DATA_URL")
	logging.DebugCtx(ctx, "Flight data URL configured", "url", flightDataURL)

	span.SetAttributes(
		attribute.String("http.url", flightDataURL),
		attribute.String("http.method", "GET"),
	)

	// Create HTTP request with context for automatic tracing via otelhttp
	req, err := http.NewRequestWithContext(ctx, "GET", flightDataURL, nil)
		if err != nil {
		span.RecordError(err)
		logging.ErrorCtx(ctx, "Failed to create HTTP request", "error", err, "url", flightDataURL)
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "adsb2otel/1.0.0")

	start := time.Now()
	resp, err := httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		span.RecordError(err)
		logging.ErrorCtx(ctx, "Failed to fetch dump1090-fa data", "error", err, "url", flightDataURL, "duration_ms", duration.Milliseconds())
		return fmt.Errorf("failed to fetch dump1090-fa data: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	logging.DebugHTTPCtx(ctx, "GET", flightDataURL, resp.StatusCode, duration)

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("HTTP request failed with status: %s", resp.Status)
		span.RecordError(err)
		logging.ErrorCtx(ctx, "HTTP request returned non-200 status", "status_code", resp.StatusCode, "status", resp.Status)
		return err
	}

	var data models.Dump1090fa
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		span.RecordError(err)
		logging.ErrorCtx(ctx, "Failed to decode dump1090-fa data", "error", err)
		return fmt.Errorf("failed to decode dump1090-fa data: %w", err)
	}

	span.SetAttributes(
		attribute.Int("aircraft.count", len(data.Aircraft)),
		attribute.Int64("data.timestamp", int64(data.Now)),
		attribute.Int("data.messages", data.Messages),
	)

	logging.DebugCtx(ctx, "Successfully parsed flight data", "aircraft_count", len(data.Aircraft), "timestamp", data.Now, "messages", data.Messages)

	// Get logger instance
	logger := logs.GetLogger("flightdata")
	if logger == nil {
		logging.WarnCtx(ctx, "OpenTelemetry logger not initialized, skipping log emission")
		return nil
	}

	// Emit log records for each aircraft
	timestamp := time.Unix(int64(data.Now), 0)
	logsEmitted := 0

	for i, aircraft := range data.Aircraft {
		logging.DebugCtx(ctx, "Processing aircraft", "index", i, "hex", aircraft.Hex, "flight", aircraft.Flight, "lat", aircraft.Lat, "lon", aircraft.Lon, "alt_baro", aircraft.AltBaro.String())

		aircraftJSON, err := json.Marshal(aircraft)
		if err != nil {
			logging.ErrorCtx(ctx, "Failed to marshal aircraft data", "error", err, "aircraft_hex", aircraft.Hex)
			return fmt.Errorf("failed to marshal aircraft data: %w", err)
		}

		// Build attributes for the log record
		attrs := []otellog.KeyValue{
			otellog.String("service", "adsb"),
			otellog.String("aircraft.hex", aircraft.Hex),
			otellog.String("aircraft.type", aircraft.Type),
		}

		// Add optional fields as attributes
		if aircraft.Flight != "" {
			attrs = append(attrs, otellog.String("aircraft.flight", aircraft.Flight))
		}
		if aircraft.Lat != 0 {
			attrs = append(attrs, otellog.Float64("aircraft.lat", aircraft.Lat))
		}
		if aircraft.Lon != 0 {
			attrs = append(attrs, otellog.Float64("aircraft.lon", aircraft.Lon))
		}
		if aircraft.AltBaro.String() != "" {
			attrs = append(attrs, otellog.String("aircraft.alt_baro", aircraft.AltBaro.String()))
		}
		if aircraft.Squawk != "" {
			attrs = append(attrs, otellog.String("aircraft.squawk", aircraft.Squawk))
		}

		// Create log record with trace context
		record := otellog.Record{}
		record.SetTimestamp(timestamp)
		record.SetSeverity(otellog.SeverityInfo)
		record.SetBody(otellog.StringValue(string(aircraftJSON)))
		
		// Add attributes to the record
		record.AddAttributes(attrs...)

		// Emit log record
		logger.Emit(ctx, record)

		logsEmitted++
	}

	logging.DebugCtx(ctx, "Converted aircraft data to OTel log records", "entries_count", logsEmitted)

	span.SetAttributes(
		attribute.Int("otel.logs_emitted", logsEmitted),
	)

	logging.InfoCtx(ctx, "Successfully fetched and pushed aircraft data", "aircraft_count", len(data.Aircraft), "logs_emitted", logsEmitted)
	return nil
}
