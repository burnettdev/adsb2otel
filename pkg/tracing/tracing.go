package tracing

import (
	"context"
	"log"
	"os"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func InitTracing() (func(), error) {
	// Check if tracing is enabled
	if enabled := getEnv("OTEL_TRACING_ENABLED", "false"); !isTrue(enabled) {
		log.Println("OpenTelemetry tracing is disabled")
		return func() {}, nil
	}

	// Get OTLP endpoint from environment variables (shared first, then signal-specific)
	endpoint := getOTLPEndpoint()

	// Check if connection should be insecure (shared first, then signal-specific)
	insecure := getEnvBool("OTEL_EXPORTER_OTLP_INSECURE", "OTEL_EXPORTER_OTLP_TRACES_INSECURE", true)

	// Parse headers if provided (shared first, then signal-specific)
	headers := parseHeaders(getEnvWithFallback("OTEL_EXPORTER_OTLP_HEADERS", "OTEL_EXPORTER_OTLP_TRACES_HEADERS", ""))

	// Determine protocol (http or grpc) - shared first, then signal-specific
	protocol := strings.ToLower(getEnvWithFallback("OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http"))
	if protocol != "http" && protocol != "grpc" {
		log.Printf("Invalid protocol %s, defaulting to http", protocol)
		protocol = "http"
	}

	var exporter trace.SpanExporter
	var err error

	// Create exporter based on protocol
	if protocol == "grpc" {
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(endpoint),
		}

		if insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}

		if len(headers) > 0 {
			opts = append(opts, otlptracegrpc.WithHeaders(headers))
		}

		exporter, err = otlptracegrpc.New(context.Background(), opts...)
	} else {
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(endpoint),
		}

		if insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}

		if len(headers) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(headers))
		}

		exporter, err = otlptracehttp.New(context.Background(), opts...)
	}
	if err != nil {
		log.Printf("Failed to create OTLP exporter, using noop: %v", err)
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

	// Create trace provider
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
	)

	// Set global trace provider
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	log.Printf("OpenTelemetry tracing initialized successfully (protocol: %s, endpoint: %s)", protocol, endpoint)

	return func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}, nil
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

	// Fall back to traces-specific endpoint if shared is not set
	if endpoint := getEnv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", ""); endpoint != "" {
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
	// Remove grpc:// prefix if present
	endpoint = strings.TrimPrefix(endpoint, "grpc://")
	
	// Remove /v1/traces suffix if present since WithEndpoint handles the path separately
	if strings.HasSuffix(endpoint, "/v1/traces") {
		endpoint = strings.TrimSuffix(endpoint, "/v1/traces")
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
