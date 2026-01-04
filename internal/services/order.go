package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/SigNoz/ecommerce-go-app/internal/db"
	"github.com/SigNoz/ecommerce-go-app/internal/metrics"
	"github.com/SigNoz/ecommerce-go-app/internal/models"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// OrderService handles order-related operations
type OrderService struct {
	db      *db.DB
	metrics *metrics.AppMetrics
}

// NewOrderService creates a new order service
func NewOrderService(db *db.DB, metrics *metrics.AppMetrics) *OrderService {
	return &OrderService{
		db:      db,
		metrics: metrics,
	}
}

// CreateOrder creates a new order from the user's cart
func (s *OrderService) CreateOrder(ctx context.Context, userID int64, paymentMethod, currency string) (*models.Order, error) {
	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	start := time.Now()

	// Get cart items
	cartQuery := `
		SELECT ci.product_id, ci.quantity, p.price
		FROM cart_items ci
		JOIN products p ON ci.product_id = p.id
		JOIN carts c ON ci.cart_id = c.id
		WHERE c.user_id = ?
	`
	rows, err := tx.QueryContext(ctx, cartQuery, userID)
	s.metrics.RecordDBQuery(ctx, "SELECT", "cart_items", cartQuery, start, err == nil)
	if err != nil {
		s.metrics.RecordDBQuery(ctx, "SELECT", "cart_items", cartQuery, start, false)
		return nil, fmt.Errorf("failed to get cart items: %w", err)
	}

	var items []struct {
		ProductID int64
		Quantity  int
		Price     float64
	}
	var totalAmount float64

	for rows.Next() {
		var item struct {
			ProductID int64
			Quantity  int
			Price     float64
		}
		if err := rows.Scan(&item.ProductID, &item.Quantity, &item.Price); err != nil {
			return nil, fmt.Errorf("failed to scan cart item: %w", err)
		}
		items = append(items, item)
		totalAmount += item.Price * float64(item.Quantity)
	}
	rows.Close()

	if len(items) == 0 {
		return nil, fmt.Errorf("cart is empty")
	}

	// ============================================
	// GET PRODUCT CATEGORIES FOR ALL ITEMS
	// ============================================
	type itemWithCategory struct {
		productID int64
		quantity  int
		price     float64
		category  string
	}

	var itemsWithCategories []itemWithCategory

	// Query all product categories
	productIDs := make([]interface{}, len(items))
	for i, item := range items {
		productIDs[i] = item.ProductID
	}

	placeholders := make([]string, len(items))
	for i := range items {
		placeholders[i] = "?"
	}

	start = time.Now()
	catQuery := fmt.Sprintf("SELECT id, category FROM products WHERE id IN (%s)",
		strings.Join(placeholders, ","))
	catRows, err := s.db.QueryContext(ctx, catQuery, productIDs...)
	s.metrics.RecordDBQuery(ctx, "SELECT", "products", catQuery, start, err == nil)

	categoryMap := make(map[int64]string)
	if err == nil {
		for catRows.Next() {
			var id int64
			var category string
			if err := catRows.Scan(&id, &category); err == nil {
				categoryMap[id] = category
			}
		}
		catRows.Close()
	}

	// Build items with categories
	for _, item := range items {
		category := categoryMap[item.ProductID]
		if category == "" {
			category = "unknown"
		}
		itemsWithCategories = append(itemsWithCategories, itemWithCategory{
			productID: item.ProductID,
			quantity:  item.Quantity,
			price:     item.Price,
			category:  category,
		})
	}

	// ============================================
	// CREATE ORDER
	// ============================================
	start = time.Now()
	orderQuery := "INSERT INTO orders (user_id, status, payment_method, total_amount, currency) VALUES (?, 'pending', ?, ?, ?)"
	result, err := tx.ExecContext(ctx, orderQuery, userID, paymentMethod, totalAmount, currency)
	s.metrics.RecordDBQuery(ctx, "INSERT", "orders", orderQuery, start, err == nil)
	if err != nil {
		s.metrics.RecordDBQuery(ctx, "INSERT", "orders", orderQuery, start, false)
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	orderID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get order ID: %w", err)
	}

	// Create order items
	start = time.Now()
	itemQuery := "INSERT INTO order_items (order_id, product_id, quantity, price) VALUES (?, ?, ?, ?)"
	for _, item := range items {
		_, err = tx.ExecContext(ctx, itemQuery, orderID, item.ProductID, item.Quantity, item.Price)
		s.metrics.RecordDBQuery(ctx, "INSERT", "order_items", itemQuery, start, err == nil)
		if err != nil {
			s.metrics.RecordDBQuery(ctx, "INSERT", "order_items", itemQuery, start, false)
			return nil, fmt.Errorf("failed to create order item: %w", err)
		}
	}

	// Clear cart
	start = time.Now()
	cartIDQuery := "SELECT id FROM carts WHERE user_id = ?"
	var cartID int64
	err = tx.QueryRowContext(ctx, cartIDQuery, userID).Scan(&cartID)
	if err == nil {
		deleteQuery := "DELETE FROM cart_items WHERE cart_id = ?"
		_, err = tx.ExecContext(ctx, deleteQuery, cartID)
		s.metrics.RecordDBQuery(ctx, "DELETE", "cart_items", deleteQuery, start, err == nil)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Order stays "pending" as created
	// Traffic script will handle 70/30 completion via PUT /api/v1/orders/{id}/status
	log.Printf("[ORDER] Order created: order_id=%d, status=pending", orderID)

	// Get created order with UPDATED status
	order, err := s.GetOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}

	// ============================================
	// CALCULATE TOTALS PER CATEGORY
	// ============================================
	categoryRevenue := make(map[string]float64)
	categoryOrders := make(map[string]int)

	for _, item := range itemsWithCategories {
		categoryRevenue[item.category] += item.price * float64(item.quantity)
		categoryOrders[item.category]++
	}

	// ============================================
	// RECORD METRICS PER CATEGORY
	// ============================================
	for category, orderCount := range categoryOrders {
		// Record order metric WITH CATEGORY and STATUS
		orderAttrs := s.metrics.WithServiceName([]attribute.KeyValue{
			attribute.String("order_status", order.Status),
			attribute.String("payment_method", paymentMethod),
			attribute.String("product_category", category),
		})

		log.Printf("[METRICS] Recording order: category=%s, count=%d, status=%s, payment_method=%s, order_id=%d",
			category, orderCount, order.Status, order.PaymentMethod, orderID)
		s.metrics.OrdersCreated.Add(ctx, int64(orderCount), metric.WithAttributes(orderAttrs...))
		log.Printf("[METRICS] ✓ OrdersCreated metric recorded for category %s with status=%s", category, order.Status)

		// Record revenue metric WITH CATEGORY and STATUS
		amount := categoryRevenue[category]
		revenueAttrs := s.metrics.WithServiceName([]attribute.KeyValue{
			attribute.String("currency", currency),
			attribute.String("payment_method", paymentMethod),
			attribute.String("product_category", category),
			attribute.String("order_status", order.Status),
		})

		log.Printf("[METRICS] Recording revenue: amount=%.2f, currency=%s, category=%s, status=%s, payment_method=%s, order_id=%d",
			amount, currency, category, order.Status, paymentMethod, orderID)
		s.metrics.RevenueTotal.Add(ctx, amount, metric.WithAttributes(revenueAttrs...))
		log.Printf("[METRICS] ✓ RevenueTotal metric recorded for category %s (value=%.2f %s)", category, amount, currency)
	}

	log.Printf("[ORDER] Order complete: order_id=%d, total=%.2f %s, status=%s, categories=%d, items=%d",
		orderID, totalAmount, currency, order.Status, len(categoryRevenue), len(itemsWithCategories))

	return order, nil
}

// GetOrder returns an order by ID
func (s *OrderService) GetOrder(ctx context.Context, orderID int64) (*models.Order, error) {
	start := time.Now()

	query := "SELECT id, user_id, status, payment_method, total_amount, currency, created_at, updated_at FROM orders WHERE id = ?"
	var order models.Order
	err := s.db.QueryRowContext(ctx, query, orderID).Scan(
		&order.ID, &order.UserID, &order.Status, &order.PaymentMethod,
		&order.TotalAmount, &order.Currency, &order.CreatedAt, &order.UpdatedAt,
	)

	s.metrics.RecordDBQuery(ctx, "SELECT", "orders", query, start, err == nil)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("order not found")
	}
	if err != nil {
		s.metrics.RecordDBQuery(ctx, "SELECT", "orders", query, start, false)
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	return &order, nil
}

// ListUserOrders returns all orders for a user
func (s *OrderService) ListUserOrders(ctx context.Context, userID int64) ([]models.Order, error) {
	start := time.Now()
	query := "SELECT id, user_id, status, payment_method, total_amount, currency, created_at, updated_at FROM orders WHERE user_id = ? ORDER BY created_at DESC"
	rows, err := s.db.QueryContext(ctx, query, userID)
	s.metrics.RecordDBQuery(ctx, "SELECT", "orders", query, start, err == nil)
	if err != nil {
		s.metrics.RecordDBQuery(ctx, "SELECT", "orders", query, start, false)
		return nil, fmt.Errorf("failed to query orders: %w", err)
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var order models.Order
		if err := rows.Scan(
			&order.ID, &order.UserID, &order.Status, &order.PaymentMethod,
			&order.TotalAmount, &order.Currency, &order.CreatedAt, &order.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan order: %w", err)
		}
		orders = append(orders, order)
	}

	return orders, rows.Err()
}

// UpdateOrderStatus updates the status of an order
func (s *OrderService) UpdateOrderStatus(ctx context.Context, orderID int64, status string) error {
	start := time.Now()

	// Validate status
	validStatuses := map[string]bool{
		"pending":    true,
		"processing": true,
		"completed":  true,
		"cancelled":  true,
		"shipped":    true,
		"delivered":  true,
	}
	if !validStatuses[status] {
		return fmt.Errorf("invalid status: %s", status)
	}

	query := "UPDATE orders SET status = ?, updated_at = NOW() WHERE id = ?"
	result, err := s.db.ExecContext(ctx, query, status, orderID)
	s.metrics.RecordDBQuery(ctx, "UPDATE", "orders", query, start, err == nil)
	if err != nil {
		s.metrics.RecordDBQuery(ctx, "UPDATE", "orders", query, start, false)
		return fmt.Errorf("failed to update order status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("order not found")
	}

	// ============================================
	// RECORD METRICS WHEN ORDER IS COMPLETED
	// ============================================
	if status == "completed" {
		// Fetch the completed order
		order, err := s.GetOrder(ctx, orderID)
		if err != nil {
			log.Printf("[WARNING] Could not fetch order for metrics: %v", err)
			return nil
		}

		// Get order items with categories for this order
		itemQuery := `
        SELECT oi.product_id, oi.quantity, oi.price, p.category
        FROM order_items oi
        JOIN products p ON oi.product_id = p.id
        WHERE oi.order_id = ?
    `
		itemRows, err := s.db.QueryContext(ctx, itemQuery, orderID)
		if err != nil {
			log.Printf("[WARNING] Could not fetch order items for metrics: %v", err)
			return nil
		}
		defer itemRows.Close()

		// Build category-wise revenue
		categoryRevenue := make(map[string]float64)
		categoryOrders := make(map[string]int)

		for itemRows.Next() {
			var productID int64
			var quantity int
			var price float64
			var category string

			if err := itemRows.Scan(&productID, &quantity, &price, &category); err != nil {
				log.Printf("[WARNING] Failed to scan order item: %v", err)
				continue
			}

			categoryRevenue[category] += price * float64(quantity)
			categoryOrders[category]++
		}

		// Record metrics per category with COMPLETED status
		for category, orderCount := range categoryOrders {
			// Record orders_created_total with status="completed"
			orderAttrs := s.metrics.WithServiceName([]attribute.KeyValue{
				attribute.String("order_status", "completed"),
				attribute.String("payment_method", order.PaymentMethod),
				attribute.String("product_category", category),
			})

			log.Printf("[METRICS] Recording completed order: order_id=%d, status=completed, category=%s, payment_method=%s",
				orderID, category, order.PaymentMethod)
			s.metrics.OrdersCreated.Add(ctx, int64(orderCount), metric.WithAttributes(orderAttrs...))

			// Record revenue_total with status="completed"
			amount := categoryRevenue[category]
			revenueAttrs := s.metrics.WithServiceName([]attribute.KeyValue{
				attribute.String("currency", order.Currency),
				attribute.String("payment_method", order.PaymentMethod),
				attribute.String("product_category", category),
				attribute.String("order_status", "completed"),
			})

			log.Printf("[METRICS] Recording completed order revenue: order_id=%d, amount=%.2f, category=%s",
				orderID, amount, category)
			s.metrics.RevenueTotal.Add(ctx, amount, metric.WithAttributes(revenueAttrs...))
		}
	}

	return nil
}
