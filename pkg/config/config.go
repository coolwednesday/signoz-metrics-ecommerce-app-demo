package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds application configuration from environment variables
type Config struct {
	// Application
	AppPort string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	// OpenTelemetry
	OTELExporterOTLPEndpoint  string
	OTELExporterOTLPProtocol  string
	OTELExporterOTLPHeaders   string // For SigNoz Cloud: signoz-ingestion-key=<key>
	OTELExporterOTLPInsecure  bool   // true for http://, false for https://
	OTELServiceName           string
	OTELServiceVersion        string
	OTELDeploymentEnvironment string
	OTELResourceAttributes    string
}

// LoadConfig loads configuration from .env file and environment variables with defaults
func LoadConfig() *Config {
	// Load .env file if it exists (ignore error if file doesn't exist)
	if err := godotenv.Load(); err != nil {
		// .env file is optional, so we only log if there's an actual error (not just file not found)
		if _, ok := err.(*os.PathError); !ok {
			log.Printf("Warning: Error loading .env file: %v", err)
		}
	}

	return &Config{
		// Application
		AppPort: getEnv("APP_PORT", "8080"),

		// Database
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: getEnv("DB_PASSWORD", "password"),
		DBName:     getEnv("DB_NAME", "ecommerce"),

		// OpenTelemetry
		OTELExporterOTLPEndpoint:  getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318"),
		OTELExporterOTLPProtocol:  getEnv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf"),
		OTELExporterOTLPHeaders:   getEnv("OTEL_EXPORTER_OTLP_HEADERS", ""),        // For SigNoz Cloud: signoz-ingestion-key=<key>
		OTELExporterOTLPInsecure:  getEnvBool("OTEL_EXPORTER_OTLP_INSECURE", true), // Default true for local dev
		OTELServiceName:           getEnv("OTEL_SERVICE_NAME", "ecommerce-go-app"),
		OTELServiceVersion:        getEnv("OTEL_SERVICE_VERSION", "1.0.0"),
		OTELDeploymentEnvironment: getEnv("OTEL_DEPLOYMENT_ENVIRONMENT", "development"),
		OTELResourceAttributes:    getEnv("OTEL_RESOURCE_ATTRIBUTES", ""),
	}
}

// GetDSN returns the MySQL DSN string
func (c *Config) GetDSN() string {
	return c.DBUser + ":" + c.DBPassword + "@tcp(" + c.DBHost + ":" + c.DBPort + ")/" + c.DBName + "?parseTime=true&charset=utf8mb4"
}

// GetAppPortInt returns the application port as an integer
func (c *Config) GetAppPortInt() int {
	port, err := strconv.Atoi(c.AppPort)
	if err != nil {
		return 8080
	}
	return port
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if value == "true" || value == "1" || value == "yes" {
			return true
		}
		return false
	}
	return defaultValue
}
