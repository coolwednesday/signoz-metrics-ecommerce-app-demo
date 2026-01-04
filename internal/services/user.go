package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/SigNoz/ecommerce-go-app/internal/db"
	"github.com/SigNoz/ecommerce-go-app/internal/metrics"
	"github.com/SigNoz/ecommerce-go-app/internal/models"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// UserService handles user-related operations
type UserService struct {
	db      *db.DB
	metrics *metrics.AppMetrics
}

// NewUserService creates a new user service
func NewUserService(db *db.DB, metrics *metrics.AppMetrics) *UserService {
	return &UserService{
		db:      db,
		metrics: metrics,
	}
}

// CreateUser creates a new user
func (s *UserService) CreateUser(ctx context.Context, email, name string) (*models.User, error) {
	start := time.Now()

	query := "INSERT INTO users (email, name) VALUES (?, ?)"
	result, err := s.db.ExecContext(ctx, query, email, name)
	s.metrics.RecordDBQuery(ctx, "INSERT", "users", query, start, err == nil)
	if err != nil {
		s.metrics.RecordDBQuery(ctx, "INSERT", "users", query, start, false)
		// Check for duplicate entry error (MySQL Error 1062)
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, fmt.Errorf("user already exists")
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get user ID: %w", err)
	}

	// Update active users count - include user_id to track unique users
	s.metrics.ActiveUsersCount.Record(ctx, 1, metric.WithAttributes(s.metrics.WithServiceName([]attribute.KeyValue{
		attribute.String("session_type", "authenticated"),
		attribute.Int64("user_id", id),
	})...))

	return &models.User{
		ID:        id,
		Email:     email,
		Name:      name,
		CreatedAt: time.Now(),
	}, nil
}

// GetUser returns a user by ID
func (s *UserService) GetUser(ctx context.Context, id int64) (*models.User, error) {
	start := time.Now()

	query := "SELECT id, email, name, created_at FROM users WHERE id = ?"
	var user models.User
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.Name, &user.CreatedAt,
	)

	s.metrics.RecordDBQuery(ctx, "SELECT", "users", query, start, err == nil)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		s.metrics.RecordDBQuery(ctx, "SELECT", "users", query, start, false)
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// GetUserByEmail returns a user by email
func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	start := time.Now()

	query := "SELECT id, email, name, created_at FROM users WHERE email = ?"
	var user models.User
	err := s.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &user.Name, &user.CreatedAt,
	)

	s.metrics.RecordDBQuery(ctx, "SELECT", "users", query, start, err == nil)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		s.metrics.RecordDBQuery(ctx, "SELECT", "users", query, start, false)
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}
