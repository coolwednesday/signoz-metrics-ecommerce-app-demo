package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SigNoz/ecommerce-go-app/internal/api"
	"github.com/SigNoz/ecommerce-go-app/internal/db"
	"github.com/SigNoz/ecommerce-go-app/internal/metrics"
	"github.com/SigNoz/ecommerce-go-app/internal/services"
	"github.com/SigNoz/ecommerce-go-app/pkg/config"
	"github.com/gorilla/mux"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Initialize OpenTelemetry metrics
	ctx := context.Background()
	appMetrics, meterProvider, err := metrics.InitMetrics(ctx, cfg)
	if err != nil {
		log.Fatalf("Failed to initialize metrics: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := meterProvider.Shutdown(shutdownCtx); err != nil {
			log.Printf("Error shutting down meter provider: %v", err)
		}
	}()

	// Initialize database
	database, err := db.NewDB(cfg.GetDSN(), meterProvider.Meter(cfg.OTELServiceName), cfg.OTELServiceName)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// Initialize schema
	schemaSQL, err := os.ReadFile("schema.sql")
	if err != nil {
		log.Printf("Warning: Could not read schema.sql: %v", err)
		log.Println("Assuming database schema already exists")
	} else {
		if err := database.InitSchema(ctx, string(schemaSQL)); err != nil {
			log.Printf("Warning: Could not initialize schema: %v", err)
			log.Println("Assuming database schema already exists")
		}
	}

	// Initialize services
	productService := services.NewProductService(database, appMetrics)
	cartService := services.NewCartService(database, appMetrics)
	orderService := services.NewOrderService(database, appMetrics)
	userService := services.NewUserService(database, appMetrics)

	// Initialize app
	app := api.NewApp(cfg, database, appMetrics, productService, cartService, orderService, userService)

	// Setup router
	router := mux.NewRouter()
	app.SetupRoutes(router)

	// Apply middleware (now handled in SetupRoutes, but keeping global ones if needed)
	// Note: SetupRoutes in handlers.go already applies these, so we can remove them here
	// or keep them if they are global. Let's rely on SetupRoutes.

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.AppPort),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server starting on port %s", cfg.AppPort)
		log.Printf("OTLP endpoint: %s", cfg.OTELExporterOTLPEndpoint)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
