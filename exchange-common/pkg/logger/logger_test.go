package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func decodeLastLogLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	lines := strings.Split(buf.String(), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(lines[i]), &payload); err != nil {
			t.Fatalf("failed to decode log line: %v", err)
		}
		return payload
	}

	t.Fatal("no log lines found")
	return nil
}

func TestWithContextInjectsFields(t *testing.T) {
	var buf bytes.Buffer
	log := New("matching", &buf)

	ctx := ContextWithTraceID(context.Background(), "trace-123")
	ctx = ContextWithSpanID(ctx, "span-456")

	log.WithContext(ctx).Info("order book updated")

	payload := decodeLastLogLine(t, &buf)

	if payload["service"] != "matching" {
		t.Fatalf("expected service to be injected, got %v", payload["service"])
	}
	if payload["traceID"] != "trace-123" {
		t.Fatalf("expected traceID to be injected, got %v", payload["traceID"])
	}
	if payload["spanID"] != "span-456" {
		t.Fatalf("expected spanID to be injected, got %v", payload["spanID"])
	}
	if payload["timestamp"] == nil {
		t.Fatalf("expected timestamp to be injected")
	}
	if payload["level"] != "info" {
		t.Fatalf("expected level to be info, got %v", payload["level"])
	}
	if payload["message"] != "order book updated" {
		t.Fatalf("expected message to match, got %v", payload["message"])
	}
}

func TestWithContextDefaultsToEmptyIDs(t *testing.T) {
	var buf bytes.Buffer
	log := New("gateway", &buf)

	log.WithContext(context.Background()).Debug("ping")

	payload := decodeLastLogLine(t, &buf)

	if payload["traceID"] != "" {
		t.Fatalf("expected empty traceID, got %v", payload["traceID"])
	}
	if payload["spanID"] != "" {
		t.Fatalf("expected empty spanID, got %v", payload["spanID"])
	}
	if payload["level"] != "debug" {
		t.Fatalf("expected level to be debug, got %v", payload["level"])
	}
}

func TestLevels(t *testing.T) {
	tests := []struct {
		name  string
		logFn func(*Logger)
		want  string
	}{
		{
			name: "warn",
			logFn: func(l *Logger) {
				l.Warn("warning")
			},
			want: "warn",
		},
		{
			name: "error",
			logFn: func(l *Logger) {
				l.Error("failure")
			},
			want: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := New("clearing", &buf)

			tt.logFn(log)

			payload := decodeLastLogLine(t, &buf)
			if payload["level"] != tt.want {
				t.Fatalf("expected level %s, got %v", tt.want, payload["level"])
			}
		})
	}
}

func TestContextHelpers(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "trace-x")
	ctx = ContextWithSpanID(ctx, "span-y")

	if got := TraceIDFromContext(ctx); got != "trace-x" {
		t.Fatalf("expected trace id trace-x, got %q", got)
	}
	if got := SpanIDFromContext(ctx); got != "span-y" {
		t.Fatalf("expected span id span-y, got %q", got)
	}

	typedCtx := context.WithValue(context.Background(), traceIDKey, 123)
	if got := TraceIDFromContext(typedCtx); got != "" {
		t.Fatalf("expected empty trace id for non-string, got %q", got)
	}
	if got := SpanIDFromContext(nil); got != "" {
		t.Fatalf("expected empty span id for nil context, got %q", got)
	}
}

func TestNewWithNilWriter(t *testing.T) {
	log := New("wallet", nil)
	if log == nil {
		t.Fatal("expected logger instance")
	}
}
