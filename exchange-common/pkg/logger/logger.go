package logger

import (
	"context"
	"io"
	"os"

	"github.com/rs/zerolog"
)

type ctxKey string

const (
	traceIDKey ctxKey = "traceID"
	spanIDKey  ctxKey = "spanID"
)

func init() {
	zerolog.TimestampFieldName = "timestamp"
}

type Logger struct {
	logger zerolog.Logger
}

func New(service string, w io.Writer) *Logger {
	if w == nil {
		w = os.Stdout
	}

	l := zerolog.New(w).With().
		Timestamp().
		Str("service", service).
		Logger()

	return &Logger{logger: l}
}

func (l *Logger) WithContext(ctx context.Context) *Logger {
	traceID := TraceIDFromContext(ctx)
	spanID := SpanIDFromContext(ctx)

	updated := l.logger.With().
		Str("traceID", traceID).
		Str("spanID", spanID).
		Logger()

	return &Logger{logger: updated}
}

func (l *Logger) Debug(msg string) {
	l.logger.Debug().Msg(msg)
}

func (l *Logger) Info(msg string) {
	l.logger.Info().Msg(msg)
}

func (l *Logger) Warn(msg string) {
	l.logger.Warn().Msg(msg)
}

func (l *Logger) Error(msg string) {
	l.logger.Error().Msg(msg)
}

func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

func ContextWithSpanID(ctx context.Context, spanID string) context.Context {
	return context.WithValue(ctx, spanIDKey, spanID)
}

func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	value, ok := ctx.Value(traceIDKey).(string)
	if !ok {
		return ""
	}

	return value
}

func SpanIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	value, ok := ctx.Value(spanIDKey).(string)
	if !ok {
		return ""
	}

	return value
}
