package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/go-sql-driver/mysql"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// DB wraps the database connection with metrics
type DB struct {
	*sql.DB
	meter            metric.Meter
	connectionActive metric.Int64Gauge
	connectionIdle   metric.Int64Gauge
	serviceName      string
}

// NewDB creates a new database connection with OpenTelemetry instrumentation
func NewDB(dsn string, meter metric.Meter, serviceName string) (*DB, error) {
	// Register otelsql wrapper for MySQL driver
	driverName, err := otelsql.Register("mysql",
		otelsql.WithAttributes(
			attribute.String("db.system", "mysql"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register otelsql: %w", err)
	}

	// Open database connection
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create metrics for connection pool
	connectionActive, err := meter.Int64Gauge(
		"db.client.connections.active",
		metric.WithDescription("Number of active database connections"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection active gauge: %w", err)
	}

	connectionIdle, err := meter.Int64Gauge(
		"db.client.connections.idle",
		metric.WithDescription("Number of idle database connections"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection idle gauge: %w", err)
	}

	dbWrapper := &DB{
		DB:               db,
		meter:            meter,
		connectionActive: connectionActive,
		connectionIdle:   connectionIdle,
		serviceName:      serviceName,
	}

	// Register otelsql's built-in stats reporting
	if err := otelsql.RegisterDBStatsMetrics(db, otelsql.WithAttributes(
		attribute.String("db.system", "mysql"),
		attribute.String("service.name", serviceName),
	)); err != nil {
		log.Printf("Warning: failed to register otelsql stats metrics: %v", err)
	}

	return dbWrapper, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}

// InitSchema initializes the database schema
// It splits the SQL into individual statements and executes them one by one
func (db *DB) InitSchema(ctx context.Context, schemaSQL string) error {
	// Split SQL into individual statements
	// Remove comments and empty lines, then split by semicolon
	statements := splitSQLStatements(schemaSQL)

	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		// Skip comments
		if strings.HasPrefix(stmt, "--") {
			continue
		}

		_, err := db.ExecContext(ctx, stmt)
		if err != nil {
			return fmt.Errorf("failed to execute statement %d: %w\nStatement: %s", i+1, err, stmt)
		}
	}

	log.Println("Database schema initialized successfully")
	return nil
}

// splitSQLStatements splits a SQL string into individual statements
func splitSQLStatements(sql string) []string {
	// Remove comments (lines starting with --)
	lines := strings.Split(sql, "\n")
	var cleanedLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "--") {
			cleanedLines = append(cleanedLines, line)
		}
	}

	// Join and split by semicolon
	cleanedSQL := strings.Join(cleanedLines, "\n")
	statements := strings.Split(cleanedSQL, ";")

	// Filter out empty statements
	var result []string
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt != "" {
			result = append(result, stmt)
		}
	}

	return result
}
