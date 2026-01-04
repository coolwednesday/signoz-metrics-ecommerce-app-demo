package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/SigNoz/ecommerce-go-app/internal/db"
	"github.com/SigNoz/ecommerce-go-app/internal/metrics"
	"github.com/SigNoz/ecommerce-go-app/internal/models"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// CartService handles cart-related operations
type CartService struct {
	db      *db.DB
	metrics *metrics.AppMetrics
}

// NewCartService creates a new cart service
func NewCartService(db *db.DB, metrics *metrics.AppMetrics) *CartService {
	cs := &CartService{
		db:      db,
		metrics: metrics,
	}
	// Start monitoring active carts
	go cs.monitorActiveCarts()
	return cs
}

// monitorActiveCarts periodically updates active carts count
func (s *CartService) monitorActiveCarts() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		query := "SELECT COUNT(DISTINCT c.id) FROM carts c INNER JOIN cart_items ci ON c.id = ci.cart_id"
		start := time.Now()
		var count int
		err := s.db.QueryRowContext(ctx, query).Scan(&count)
		s.metrics.RecordDBQuery(ctx, "SELECT", "carts", query, start, err == nil)
		if err == nil {
			s.metrics.ActiveCartsCount.Record(ctx, int64(count), metric.WithAttributes(s.metrics.WithServiceName([]attribute.KeyValue{})...))
		}
	}
}

// GetOrCreateCart gets or creates a cart for a user
func (s *CartService) GetOrCreateCart(ctx context.Context, userID int64) (*models.Cart, error) {
	start := time.Now()

	// Try to get existing cart
	query := "SELECT id, user_id, created_at, updated_at FROM carts WHERE user_id = ? LIMIT 1"
	var cart models.Cart
	err := s.db.QueryRowContext(ctx, query, userID).Scan(
		&cart.ID, &cart.UserID, &cart.CreatedAt, &cart.UpdatedAt,
	)

	s.metrics.RecordDBQuery(ctx, "SELECT", "carts", query, start, err == nil || err == sql.ErrNoRows)

	if err == sql.ErrNoRows {
		// Create new cart
		start = time.Now()
		insertQuery := "INSERT INTO carts (user_id) VALUES (?)"
		result, err := s.db.ExecContext(ctx, insertQuery, userID)
		if err != nil {
			s.metrics.RecordDBQuery(ctx, "INSERT", "carts", insertQuery, start, false)
			return nil, fmt.Errorf("failed to create cart: %w", err)
		}

		s.metrics.RecordDBQuery(ctx, "INSERT", "carts", insertQuery, start, err == nil)

		id, err := result.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("failed to get cart ID: %w", err)
		}

		cart.ID = id
		cart.UserID = userID
		cart.CreatedAt = time.Now()
		cart.UpdatedAt = time.Now()
	} else if err != nil {
		s.metrics.RecordDBQuery(ctx, "SELECT", "carts", query, start, false)
		return nil, fmt.Errorf("failed to get cart: %w", err)
	}

	return &cart, nil
}

// AddToCart adds an item to the cart
func (s *CartService) AddToCart(ctx context.Context, userID int64, productID int64, quantity int) error {
	cart, err := s.GetOrCreateCart(ctx, userID)
	if err != nil {
		return err
	}

	// Verify product exists
	var exists bool
	checkProductQuery := "SELECT EXISTS(SELECT 1 FROM products WHERE id = ?)"
	if err := s.db.QueryRowContext(ctx, checkProductQuery, productID).Scan(&exists); err != nil {
		return fmt.Errorf("failed to verify product: %w", err)
	}
	if !exists {
		return fmt.Errorf("product not found")
	}

	start := time.Now()

	// Check if item already exists in cart
	checkQuery := "SELECT id, quantity FROM cart_items WHERE cart_id = ? AND product_id = ?"
	var existingID int64
	var existingQty int
	err = s.db.QueryRowContext(ctx, checkQuery, cart.ID, productID).Scan(&existingID, &existingQty)
	s.metrics.RecordDBQuery(ctx, "SELECT", "cart_items", checkQuery, start, err == nil || err == sql.ErrNoRows)

	if err == sql.ErrNoRows {
		// Insert new item
		start = time.Now()
		insertQuery := "INSERT INTO cart_items (cart_id, product_id, quantity) VALUES (?, ?, ?)"
		_, err = s.db.ExecContext(ctx, insertQuery, cart.ID, productID, quantity)
		s.metrics.RecordDBQuery(ctx, "INSERT", "cart_items", insertQuery, start, err == nil)
		if err != nil {
			s.metrics.RecordDBQuery(ctx, "INSERT", "cart_items", insertQuery, start, false)
			return fmt.Errorf("failed to add item to cart: %w", err)
		}
	} else if err != nil {
		s.metrics.RecordDBQuery(ctx, "SELECT", "cart_items", checkQuery, start, false)
		return fmt.Errorf("failed to check cart item: %w", err)
	} else {
		// Update existing item
		start = time.Now()
		updateQuery := "UPDATE cart_items SET quantity = quantity + ?, updated_at = NOW() WHERE id = ?"
		_, err = s.db.ExecContext(ctx, updateQuery, quantity, existingID)
		s.metrics.RecordDBQuery(ctx, "UPDATE", "cart_items", updateQuery, start, err == nil)
		if err != nil {
			s.metrics.RecordDBQuery(ctx, "UPDATE", "cart_items", updateQuery, start, false)
			return fmt.Errorf("failed to update cart item: %w", err)
		}
	}

	// Update cart items count gauge
	s.updateCartItemsCount(ctx, cart.ID)

	return nil
}

// RemoveFromCart removes an item from the cart
func (s *CartService) RemoveFromCart(ctx context.Context, userID int64, productID int64) error {
	cart, err := s.GetOrCreateCart(ctx, userID)
	if err != nil {
		return err
	}

	start := time.Now()

	query := "DELETE FROM cart_items WHERE cart_id = ? AND product_id = ?"
	_, err = s.db.ExecContext(ctx, query, cart.ID, productID)
	s.metrics.RecordDBQuery(ctx, "DELETE", "cart_items", query, start, err == nil)
	if err != nil {
		s.metrics.RecordDBQuery(ctx, "DELETE", "cart_items", query, start, false)
		return fmt.Errorf("failed to remove item from cart: %w", err)
	}

	// Update cart items count gauge
	s.updateCartItemsCount(ctx, cart.ID)

	return nil
}

// GetCart returns the cart with all items
func (s *CartService) GetCart(ctx context.Context, userID int64) (*models.CartResponse, error) {
	cart, err := s.GetOrCreateCart(ctx, userID)
	if err != nil {
		return nil, err
	}

	start := time.Now()

	query := `
		SELECT ci.id, ci.cart_id, ci.product_id, ci.quantity, ci.created_at, ci.updated_at,
		       p.price
		FROM cart_items ci
		JOIN products p ON ci.product_id = p.id
		WHERE ci.cart_id = ?
	`
	rows, err := s.db.QueryContext(ctx, query, cart.ID)
	s.metrics.RecordDBQuery(ctx, "SELECT", "cart_items", query, start, err == nil)
	if err != nil {
		s.metrics.RecordDBQuery(ctx, "SELECT", "cart_items", query, start, false)
		return nil, fmt.Errorf("failed to get cart items: %w", err)
	}
	defer rows.Close()

	var items []models.CartItem
	var total float64
	for rows.Next() {
		var item models.CartItem
		var price float64
		if err := rows.Scan(&item.ID, &item.CartID, &item.ProductID, &item.Quantity, &item.CreatedAt, &item.UpdatedAt, &price); err != nil {
			return nil, fmt.Errorf("failed to scan cart item: %w", err)
		}
		items = append(items, item)
		total += price * float64(item.Quantity)
	}

	// Update cart items count gauge
	s.updateCartItemsCount(ctx, cart.ID)

	return &models.CartResponse{
		Cart:  cart,
		Items: items,
		Total: total,
	}, rows.Err()
}

// updateCartItemsCount updates the cart items count gauge metric
func (s *CartService) updateCartItemsCount(ctx context.Context, cartID int64) {
	start := time.Now()

	query := "SELECT COUNT(*) FROM cart_items WHERE cart_id = ?"
	var count int
	err := s.db.QueryRowContext(ctx, query, cartID).Scan(&count)
	s.metrics.RecordDBQuery(ctx, "SELECT", "cart_items", query, start, err == nil)

	if err == nil {
		// Get user_id from cart
		var userID int64
		userQuery := "SELECT user_id FROM carts WHERE id = ?"
		if err := s.db.QueryRowContext(ctx, userQuery, cartID).Scan(&userID); err == nil {
			cartAttrs := s.metrics.WithServiceName([]attribute.KeyValue{
				attribute.Int64("user_id", userID),
			})
			log.Printf("[METRICS] Recording cart items count: user_id=%d, cart_id=%d, count=%d",
				userID, cartID, count)
			s.metrics.CartItemsCount.Record(ctx, int64(count), metric.WithAttributes(cartAttrs...))
		}
	}
}
