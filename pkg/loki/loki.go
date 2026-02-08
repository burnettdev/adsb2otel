package loki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/burnettdev/adsb2otel/pkg/logging"
)

type Client struct {
	url      string
	client   *http.Client
	tenantID string
	password string
	tracer   trace.Tracer
}

func NewClient(url string) *Client {
	logging.DebugCall("NewClient", "url", url)

	client := &Client{
		url: url,
		client: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
			Timeout:   10 * time.Second,
		},
		tracer: otel.Tracer("loki-client"),
	}

	logging.Debug("Loki client created", "url", url, "timeout", "10s", "auth", false)
	return client
}

func NewClientWithAuth(url, tenantID, password string) *Client {
	logging.DebugCall("NewClientWithAuth", "url", url, "tenant_id", tenantID, "password_set", password != "")

	client := &Client{
		url:      url,
		tenantID: tenantID,
		password: password,
		client: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
			Timeout:   10 * time.Second,
		},
		tracer: otel.Tracer("loki-client"),
	}

	logging.Debug("Loki client created with auth", "url", url, "tenant_id", tenantID, "timeout", "10s", "auth", true)
	return client
}

type LogEntry struct {
	Timestamp time.Time
	Labels    map[string]string
	Line      string
}

func (c *Client) PushLogs(ctx context.Context, entries []LogEntry) error {
	ctx, span := c.tracer.Start(ctx, "loki.push_logs",
		trace.WithAttributes(
			attribute.Int("entries_count", len(entries)),
			attribute.Bool("auth.enabled", c.tenantID != "" && c.password != ""),
		),
	)
	defer span.End()

	logging.DebugCall("PushLogs", "entries_count", len(entries))

	if len(entries) == 0 {
		logging.Debug("No entries to push, skipping")
		return nil
	}

	streams := make([]map[string]interface{}, 0)
	for _, entry := range entries {
		stream := map[string]interface{}{
			"stream": entry.Labels,
			"values": [][]string{
				{fmt.Sprintf("%d", entry.Timestamp.UnixNano()), entry.Line},
			},
		}
		streams = append(streams, stream)
	}

	payload := map[string]interface{}{
		"streams": streams,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		span.RecordError(err)
		logging.Error("Failed to marshal Loki payload", "error", err, "entries_count", len(entries))
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	span.SetAttributes(
		attribute.Int("payload.size_bytes", len(data)),
		attribute.Int("streams_count", len(streams)),
	)

	logging.Debug("Loki payload marshaled", "payload_size", len(data), "streams_count", len(streams))

	url := c.url + "/loki/api/v1/push"

	span.SetAttributes(
		attribute.String("http.url", url),
		attribute.String("http.method", "POST"),
	)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		span.RecordError(err)
		logging.Error("Failed to create HTTP request", "error", err, "url", url)
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "adsb2otel/1.0.0")

	if c.tenantID != "" && c.password != "" {
		req.SetBasicAuth(c.tenantID, c.password)
		span.SetAttributes(
			attribute.String("auth.username", c.tenantID),
		)
		logging.Debug("Added basic authentication to request", "tenant_id", c.tenantID)
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		span.RecordError(err)
		logging.Error("HTTP request failed", "error", err, "url", url, "duration_ms", duration.Milliseconds())
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
	)
	logging.DebugHTTP("POST", url, resp.StatusCode, duration, "entries_count", len(entries))

	if resp.StatusCode == http.StatusUnauthorized {
		err := fmt.Errorf("authentication failed: %s", resp.Status)
		span.RecordError(err)
		logging.Error("Authentication failed", "status", resp.Status, "tenant_id", c.tenantID)
		return err
	}

	if resp.StatusCode >= 400 {
		err := fmt.Errorf("request failed with status: %s", resp.Status)
		span.RecordError(err)
		logging.Error("HTTP request failed with bad status", "status", resp.Status, "status_code", resp.StatusCode)
		return err
	}

	logging.Debug("Successfully pushed logs to Loki", "entries_count", len(entries), "status_code", resp.StatusCode)
	return nil
}
