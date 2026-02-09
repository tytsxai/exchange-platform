package tracing

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	ServiceName string
	Endpoint    string // Jaeger endpoint
	Enabled     bool
	SampleRate  float64 // 0.0-1.0
}

const (
	httpTraceHeader = "X-Trace-ID"
	redisTraceField = "_traceId"
	defaultSpanName = "request"
	tracerName      = "exchange-common/tracing"
	unknownService  = "unknown-service"
)

type ctxKeyTraceID struct{}

var tracingEnabled atomic.Bool

func Init(cfg Config) (shutdown func(context.Context) error, err error) {
	if !cfg.Enabled {
		tracingEnabled.Store(false)
		otel.SetTracerProvider(trace.NewNoopTracerProvider())
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
		return func(context.Context) error { return nil }, nil
	}

	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = unknownService
	}

	sampleRate := cfg.SampleRate
	switch {
	case sampleRate <= 0:
		sampleRate = 0
	case sampleRate >= 1:
		sampleRate = 1
	}

	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(cfg.Endpoint)))
	if err != nil {
		return nil, err
	}

	res, err := sdkresource.New(
		context.Background(),
		sdkresource.WithAttributes(
			attribute.String("service.name", serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRate))),
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	tracingEnabled.Store(true)

	return tp.Shutdown, nil
}

// HTTPMiddleware HTTP请求追踪中间件
func HTTPMiddleware(next http.Handler) http.Handler {
	if !tracingEnabled.Load() {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := ExtractHTTP(r.Context(), r)

		spanName := defaultSpanName
		if r.Method != "" && r.URL != nil {
			spanName = r.Method + " " + r.URL.Path
		}

		ctx, span := StartSpan(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		span.SetAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("url.path", r.URL.Path),
		)

		if traceID := TraceIDFromContext(ctx); traceID != "" {
			w.Header().Set(httpTraceHeader, traceID)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TraceIDFromContext(ctx context.Context) string {
	if !tracingEnabled.Load() {
		return ""
	}
	if ctx == nil {
		return ""
	}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		if tid := sc.TraceID().String(); tid != "" {
			return tid
		}
	}
	if v := ctx.Value(ctxKeyTraceID{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func SpanFromContext(ctx context.Context) trace.Span {
	if !tracingEnabled.Load() {
		return trace.SpanFromContext(context.Background())
	}
	return trace.SpanFromContext(ctx)
}

func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if !tracingEnabled.Load() {
		return ctx
	}

	ctx = context.WithValue(ctx, ctxKeyTraceID{}, traceID)
	if tid, ok := parseTraceID(traceID); ok {
		sc := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    tid,
			TraceFlags: trace.FlagsSampled,
			Remote:     true,
		})
		ctx = trace.ContextWithSpanContext(ctx, sc)
	}
	return ctx
}

// StartSpan 开始一个新span
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !tracingEnabled.Load() {
		return ctx, trace.SpanFromContext(context.Background())
	}
	if name == "" {
		name = defaultSpanName
	}
	return otel.Tracer(tracerName).Start(ctx, name, opts...)
}

// AddEvent 添加事件
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	if !tracingEnabled.Load() || ctx == nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetError 记录错误
func SetError(ctx context.Context, err error) {
	if !tracingEnabled.Load() || ctx == nil || err == nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// InjectHTTP 注入trace到HTTP请求头
func InjectHTTP(ctx context.Context, req *http.Request) {
	if !tracingEnabled.Load() || ctx == nil || req == nil {
		return
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		req.Header.Set(httpTraceHeader, traceID)
	}
}

// ExtractHTTP 从HTTP请求头提取trace
func ExtractHTTP(ctx context.Context, req *http.Request) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if !tracingEnabled.Load() || req == nil {
		return ctx
	}

	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(req.Header))

	if TraceIDFromContext(ctx) != "" {
		return ctx
	}
	if tid := req.Header.Get(httpTraceHeader); tid != "" {
		return ContextWithTraceID(ctx, tid)
	}
	return ctx
}

// InjectRedisStream 注入trace到Redis Stream消息
func InjectRedisStream(ctx context.Context, values map[string]interface{}) {
	if !tracingEnabled.Load() || ctx == nil || values == nil {
		return
	}
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		values[redisTraceField] = traceID
	}
}

// ExtractRedisStream 从Redis Stream消息提取trace
func ExtractRedisStream(ctx context.Context, values map[string]interface{}) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if !tracingEnabled.Load() || values == nil {
		return ctx
	}

	raw, ok := values[redisTraceField]
	if !ok || raw == nil {
		return ctx
	}

	traceID := ""
	switch v := raw.(type) {
	case string:
		traceID = v
	case []byte:
		traceID = string(v)
	default:
		traceID = fmt.Sprint(v)
	}
	if traceID == "" {
		return ctx
	}
	return ContextWithTraceID(ctx, traceID)
}

func parseTraceID(s string) (trace.TraceID, bool) {
	tid, err := trace.TraceIDFromHex(s)
	if err != nil || !tid.IsValid() {
		return trace.TraceID{}, false
	}
	return tid, true
}
