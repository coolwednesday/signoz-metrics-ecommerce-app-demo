package middleware

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/SigNoz/ecommerce-go-app/internal/metrics"
	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// MetricsMiddleware records HTTP request metrics
func MetricsMiddleware(metrics *metrics.AppMetrics) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Call next handler
			next.ServeHTTP(rw, r)

			// Calculate duration in milliseconds
			duration := time.Since(start).Milliseconds()

			// Get route pattern
			route := mux.CurrentRoute(r)
			routePattern := "unknown"
			if route != nil {
				if pathTemplate, err := route.GetPathTemplate(); err == nil {
					routePattern = pathTemplate
				}
			}

			// Record metrics
			ctx := r.Context()
			attrs := []attribute.KeyValue{
				attribute.String("http.method", r.Method),
				attribute.String("http.route", routePattern),
				attribute.Int("http.status_code", rw.statusCode),
			}

			// Record total requests
			metrics.HTTPRequestsTotal.Add(ctx, 1, metric.WithAttributes(metrics.WithServiceName(attrs)...))

			// Record error requests (4xx, 5xx)
			if rw.statusCode >= 400 {
				metrics.HTTPRequestsErrors.Add(ctx, 1, metric.WithAttributes(metrics.WithServiceName(attrs)...))
			}

			// Track active users (if user_id is present in query)
			// Include user_id as attribute so we can count distinct users
			if userID := r.URL.Query().Get("user_id"); userID != "" {
				// Parse user_id to int64 for consistent attribute type
				if uid, err := strconv.ParseInt(userID, 10, 64); err == nil {
					metrics.ActiveUsersCount.Record(ctx, 1, metric.WithAttributes(metrics.WithServiceName([]attribute.KeyValue{
						attribute.String("session_type", "active"),
						attribute.Int64("user_id", uid),
					})...))
				} else {
					// If parsing fails, use string attribute
					metrics.ActiveUsersCount.Record(ctx, 1, metric.WithAttributes(metrics.WithServiceName([]attribute.KeyValue{
						attribute.String("session_type", "active"),
						attribute.String("user_id", userID),
					})...))
				}
			}

			// Record request duration
			metrics.HTTPRequestDuration.Record(ctx, float64(duration), metric.WithAttributes(metrics.WithServiceName(attrs)...))

			// Log the request
			log.Printf("%s %s %s - %d - %dms", r.Method, routePattern, r.RemoteAddr, rw.statusCode, duration)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RequestIDMiddleware adds a request ID to the context
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), "request_id", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CORSMiddleware adds CORS headers
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ErrorHandlerMiddleware handles errors and returns JSON responses
func ErrorHandlerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func generateRequestID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}
