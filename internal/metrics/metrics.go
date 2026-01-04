package metrics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SigNoz/ecommerce-go-app/pkg/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// AppMetrics holds all application metrics
type AppMetrics struct {
	// HTTP Metrics
	HTTPRequestsTotal   metric.Int64Counter
	HTTPRequestsErrors  metric.Int64Counter
	HTTPRequestDuration metric.Float64Histogram

	// Database Metrics
	DBQueriesTotal  metric.Int64Counter
	DBQueryDuration metric.Float64Histogram

	// Business Metrics
	OrdersCreated  metric.Int64Counter
	ProductsViewed metric.Int64Counter
	CartItemsCount metric.Int64Gauge
	InventoryLevel metric.Int64Gauge
	RevenueTotal   metric.Float64Counter

	// Application Metrics
	ActiveUsersCount metric.Int64Gauge
	ActiveCartsCount metric.Int64Gauge
	CacheHits        metric.Int64Counter
	CacheMisses      metric.Int64Counter

	// Service name for adding to all metrics
	serviceName string
}

// InitMetrics initializes OpenTelemetry metrics
func InitMetrics(ctx context.Context, cfg *config.Config) (*AppMetrics, *sdkmetric.MeterProvider, error) {
	// Create resource with service information
	// Use resource.Env() to read from environment variables (OTEL_SERVICE_NAME, etc.)
	// Then merge with explicit attributes to ensure service.name is set correctly
	envRes, err := resource.New(ctx, resource.WithFromEnv())
	if err != nil {
		// If env resource fails, continue with empty resource
		envRes = resource.Empty()
	}

	// Create explicit resource with our service information
	// This takes precedence over environment variables
	explicitRes, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.OTELServiceName),
			semconv.ServiceVersion(cfg.OTELServiceVersion),
			attribute.String("deployment.environment", cfg.OTELDeploymentEnvironment),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create explicit resource: %w", err)
	}

	// Merge resources: explicit attributes take precedence over env
	res, err := resource.Merge(envRes, explicitRes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to merge resources: %w", err)
	}

	// Log the final resource attributes for debugging
	fmt.Printf("Resource attributes configured:\n")
	attrs := res.Attributes()
	for _, kv := range attrs {
		fmt.Printf("  %s = %v\n", kv.Key, kv.Value.AsInterface())
	}

	// Verify service.name is set
	var serviceNameAttr attribute.Value
	var found bool
	for _, kv := range attrs {
		if kv.Key == semconv.ServiceNameKey {
			serviceNameAttr = kv.Value
			found = true
			break
		}
	}
	if !found || serviceNameAttr.AsString() == "" {
		return nil, nil, fmt.Errorf("service.name is not set in resource attributes")
	}
	fmt.Printf("✓ Service name verified: %s\n", serviceNameAttr.AsString())

	// Create OTLP HTTP exporter
	// According to OpenTelemetry and SigNoz documentation:
	// - WithEndpoint expects host:port (without http:// or https://)
	//   For SigNoz Cloud: ingest.<region>.signoz.cloud:443
	//   For local: localhost:4318 or otel-collector:4318
	// - WithURLPath sets the OTLP metrics endpoint path
	// - WithInsecure() is used for http:// endpoints (local development)
	//   For https:// endpoints (SigNoz Cloud), omit WithInsecure()
	// - WithHeaders is used for authentication (e.g., signoz-ingestion-key)
	exporterOpts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(cfg.OTELExporterOTLPEndpoint),
		otlpmetrichttp.WithURLPath("/v1/metrics"), // OTLP HTTP metrics endpoint path
	}

	// Add headers if provided (for SigNoz Cloud authentication)
	if cfg.OTELExporterOTLPHeaders != "" {
		headers := parseHeaders(cfg.OTELExporterOTLPHeaders)
		exporterOpts = append(exporterOpts, otlpmetrichttp.WithHeaders(headers))
	}

	// Configure TLS: use insecure for http://, secure for https:// (SigNoz Cloud)
	if cfg.OTELExporterOTLPInsecure {
		exporterOpts = append(exporterOpts, otlpmetrichttp.WithInsecure())
		fmt.Printf("Metrics exporter: Using insecure HTTP connection\n")
	} else {
		fmt.Printf("Metrics exporter: Using secure HTTPS connection\n")
	}

	exporter, err := otlpmetrichttp.New(ctx, exporterOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Log exporter configuration
	fmt.Printf("\n=== Metrics Exporter Configuration ===\n")
	fmt.Printf("Endpoint: %s\n", cfg.OTELExporterOTLPEndpoint)
	fmt.Printf("Path: /v1/metrics\n")
	if cfg.OTELExporterOTLPHeaders != "" {
		headers := parseHeaders(cfg.OTELExporterOTLPHeaders)
		fmt.Printf("Headers: %d header(s) configured\n", len(headers))
	}
	fmt.Printf("Export interval: 10 seconds\n")
	fmt.Printf("Service name from config: %s\n", cfg.OTELServiceName)

	// Verify service.name in resource (re-check)
	attrs = res.Attributes()
	var serviceNameAttr2 attribute.Value
	var found2 bool
	for _, kv := range attrs {
		if kv.Key == semconv.ServiceNameKey {
			serviceNameAttr2 = kv.Value
			found2 = true
			break
		}
	}
	if found2 {
		fmt.Printf("✓ Service name in resource: %s\n", serviceNameAttr2.AsString())
		if serviceNameAttr2.AsString() != cfg.OTELServiceName {
			fmt.Printf("⚠ WARNING: Service name mismatch! Config: %s, Resource: %s\n",
				cfg.OTELServiceName, serviceNameAttr2.AsString())
		}
	} else {
		fmt.Printf("❌ ERROR: service.name NOT found in resource attributes!\n")
		fmt.Printf("Resource attributes:\n")
		for _, kv := range attrs {
			fmt.Printf("  %s = %v\n", kv.Key, kv.Value.AsInterface())
		}
	}
	fmt.Printf("=====================================\n\n")

	// Create periodic reader (exports every 10 seconds)
	reader := sdkmetric.NewPeriodicReader(exporter,
		sdkmetric.WithInterval(10*time.Second),
	)

	fmt.Printf("✓ Metrics will be exported every 10 seconds to: %s/v1/metrics\n", cfg.OTELExporterOTLPEndpoint)
	fmt.Printf("✓ Business metrics configured: orders_created_total, revenue_total, products_viewed_total, inventory_level, cart_items_count\n")
	fmt.Printf("✓ Application metrics configured: active_users_count, active_carts_count, cache_hits_total, cache_misses_total\n\n")

	// Create meter provider
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)

	// Set global meter provider
	otel.SetMeterProvider(meterProvider)

	// Get meter
	meter := meterProvider.Meter(cfg.OTELServiceName)

	// SigNoz default histogram buckets in milliseconds, expanded to 60s
	buckets := []float64{2, 4, 6, 8, 10, 50, 100, 200, 400, 800, 1000, 1400, 2000, 5000, 10000, 15000, 20000, 30000, 45000, 60000}

	// Initialize HTTP metrics
	httpRequestsTotal, err := meter.Int64Counter(
		"http.server.request.count",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create http requests counter: %w", err)
	}

	httpRequestsErrors, err := meter.Int64Counter(
		"http.server.request.error.count",
		metric.WithDescription("Total number of HTTP error requests"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create http errors counter: %w", err)
	}

	httpRequestDuration, err := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("HTTP request duration in milliseconds"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(buckets...),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create http duration histogram: %w", err)
	}

	// Initialize database metrics
	dbQueriesTotal, err := meter.Int64Counter(
		"db.client.queries.count",
		metric.WithDescription("Total number of database queries"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create db queries counter: %w", err)
	}

	dbQueryDuration, err := meter.Float64Histogram(
		"db.client.queries.duration",
		metric.WithDescription("Database query duration in milliseconds"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(buckets...),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create db duration histogram: %w", err)
	}

	// Initialize business metrics
	ordersCreated, err := meter.Int64Counter(
		"orders_created_total",
		metric.WithDescription("Total number of orders created"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create orders counter: %w", err)
	}

	productsViewed, err := meter.Int64Counter(
		"products_viewed_total",
		metric.WithDescription("Total number of product views"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create products viewed counter: %w", err)
	}

	cartItemsCount, err := meter.Int64Gauge(
		"cart_items_count",
		metric.WithDescription("Current number of items in user carts"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cart items gauge: %w", err)
	}

	inventoryLevel, err := meter.Int64Gauge(
		"inventory_level",
		metric.WithDescription("Current inventory level for products"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create inventory gauge: %w", err)
	}

	revenueTotal, err := meter.Float64Counter(
		"revenue_total",
		metric.WithDescription("Total revenue generated"),
		metric.WithUnit("USD"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create revenue counter: %w", err)
	}

	// Initialize application metrics
	activeUsersCount, err := meter.Int64Gauge(
		"active_users_count",
		metric.WithDescription("Currently active users"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create active users gauge: %w", err)
	}

	cacheHits, err := meter.Int64Counter(
		"cache_hits_total",
		metric.WithDescription("Total number of cache hits"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cache hits counter: %w", err)
	}

	cacheMisses, err := meter.Int64Counter(
		"cache_misses_total",
		metric.WithDescription("Total number of cache misses"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cache misses counter: %w", err)
	}

	activeCartsCount, err := meter.Int64Gauge(
		"active_carts_count",
		metric.WithDescription("Number of active carts with items"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create active carts gauge: %w", err)
	}

	return &AppMetrics{
		HTTPRequestsTotal:   httpRequestsTotal,
		HTTPRequestsErrors:  httpRequestsErrors,
		HTTPRequestDuration: httpRequestDuration,
		DBQueriesTotal:      dbQueriesTotal,
		DBQueryDuration:     dbQueryDuration,
		OrdersCreated:       ordersCreated,
		ProductsViewed:      productsViewed,
		CartItemsCount:      cartItemsCount,
		InventoryLevel:      inventoryLevel,
		RevenueTotal:        revenueTotal,
		ActiveUsersCount:    activeUsersCount,
		ActiveCartsCount:    activeCartsCount,
		CacheHits:           cacheHits,
		CacheMisses:         cacheMisses,
		serviceName:         cfg.OTELServiceName,
	}, meterProvider, nil
}

// WithServiceName adds service.name to attributes
func (m *AppMetrics) WithServiceName(attrs []attribute.KeyValue) []attribute.KeyValue {
	return append(attrs, attribute.String("service.name", m.serviceName))
}

// RecordDBQuery records database query metrics including the SQL statement
func (m *AppMetrics) RecordDBQuery(ctx context.Context, operation, table, statement string, start time.Time, success bool) {
	duration := time.Since(start).Milliseconds()

	status := "success"
	if !success {
		status = "error"
	}

	attrs := []attribute.KeyValue{
		attribute.String("db.operation", operation),
		attribute.String("db.sql.table", table),
		attribute.String("db.statement", statement),
		attribute.String("db.system", "mysql"),
		attribute.String("status", status),
	}

	m.DBQueriesTotal.Add(ctx, 1, metric.WithAttributes(m.WithServiceName(attrs)...))
	m.DBQueryDuration.Record(ctx, float64(duration), metric.WithAttributes(m.WithServiceName(attrs)...))
}

// parseHeaders parses header string in format "key1=value1,key2=value2"
// and returns a map of headers
func parseHeaders(headerStr string) map[string]string {
	headers := make(map[string]string)
	if headerStr == "" {
		return headers
	}

	pairs := strings.Split(headerStr, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return headers
}
