package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/SigNoz/ecommerce-go-app/internal/db"
	"github.com/SigNoz/ecommerce-go-app/internal/metrics"
	"github.com/SigNoz/ecommerce-go-app/internal/models"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// ProductCache holds cached products
type ProductCache struct {
	mu    sync.RWMutex
	items map[int64]cachedProduct
}

type cachedProduct struct {
	product models.Product
	expires time.Time
}

// ProductService handles product-related operations
type ProductService struct {
	db      *db.DB
	metrics *metrics.AppMetrics
	cache   ProductCache
}

func NewProductCache() ProductCache {
	return ProductCache{
		items: make(map[int64]cachedProduct),
	}
}

// NewProductService creates a new product service
func NewProductService(db *db.DB, metrics *metrics.AppMetrics) *ProductService {
	return &ProductService{
		db:      db,
		metrics: metrics,
		cache:   NewProductCache(),
	}
}

// ListProducts returns a paginated list of products
func (s *ProductService) ListProducts(ctx context.Context, limit, offset int) ([]models.Product, error) {
	start := time.Now()
	query := `SELECT id, name, description, price, category, sku, created_at, updated_at FROM products LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		s.metrics.RecordDBQuery(ctx, "SELECT", "products", query, start, false)
		return nil, fmt.Errorf("failed to query products: %w", err)
	}
	defer rows.Close()

	s.metrics.RecordDBQuery(ctx, "SELECT", "products", query, start, err == nil)

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Category, &p.SKU, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan product: %w", err)
		}
		products = append(products, p)
	}

	return products, rows.Err()
}

// GetProduct returns a product by ID
func (s *ProductService) GetProduct(ctx context.Context, id int64) (*models.Product, error) {
	// Check cache first
	s.cache.mu.RLock()
	if cached, exists := s.cache.items[id]; exists && time.Now().Before(cached.expires) {
		s.cache.mu.RUnlock()
		log.Printf("[METRICS] Cache HIT: product_id=%d", id)
		s.metrics.CacheHits.Add(ctx, 1, metric.WithAttributes(s.metrics.WithServiceName([]attribute.KeyValue{})...))
		log.Printf("[METRICS] CacheHits metric recorded (value=1)")

		// Record product view metric - WITH CATEGORY
		viewAttrs := s.metrics.WithServiceName([]attribute.KeyValue{
			attribute.Int64("product_id", id),
			attribute.String("product_category", cached.product.Category), // ← FIXED: ADD CATEGORY
		})
		log.Printf("[METRICS] Recording product view (cache hit): product_id=%d, product_category=%s", id, cached.product.Category)
		s.metrics.ProductsViewed.Add(ctx, 1, metric.WithAttributes(viewAttrs...))
		log.Printf("[METRICS] ProductsViewed metric recorded (cache hit)")

		return &cached.product, nil
	}
	s.cache.mu.RUnlock()

	log.Printf("[METRICS] Cache MISS: product_id=%d", id)
	s.metrics.CacheMisses.Add(ctx, 1, metric.WithAttributes(s.metrics.WithServiceName([]attribute.KeyValue{})...))
	log.Printf("[METRICS] CacheMisses metric recorded (value=1)")

	start := time.Now()
	query := `SELECT id, name, description, price, category, sku, created_at, updated_at FROM products WHERE id = ?`
	var p models.Product
	err := s.db.QueryRowContext(ctx, query, id).Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Category, &p.SKU, &p.CreatedAt, &p.UpdatedAt)

	s.metrics.RecordDBQuery(ctx, "SELECT", "products", query, start, err == nil)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("product not found")
	}
	if err != nil {
		s.metrics.RecordDBQuery(ctx, "SELECT", "products", query, start, false)
		return nil, fmt.Errorf("failed to get product: %w", err)
	}

	// Cache the product
	s.cache.mu.Lock()
	s.cache.items[id] = cachedProduct{
		product: p,
		expires: time.Now().Add(5 * time.Minute),
	}
	s.cache.mu.Unlock()

	// Record product view metric - WITH CATEGORY
	viewAttrs := s.metrics.WithServiceName([]attribute.KeyValue{
		attribute.Int64("product_id", id),
		attribute.String("product_category", p.Category), // ← FIXED: ADD CATEGORY
	})
	log.Printf("[METRICS] Recording product view (cache miss): product_id=%d, product_category=%s", id, p.Category)
	s.metrics.ProductsViewed.Add(ctx, 1, metric.WithAttributes(viewAttrs...))
	log.Printf("[METRICS] ProductsViewed metric recorded (cache miss)")

	return &p, nil
}

// GetProductInventory returns inventory level for a product
func (s *ProductService) GetProductInventory(ctx context.Context, productID int64, warehouseID string) (*models.Inventory, error) {
	start := time.Now()
	query := `SELECT id, product_id, warehouse_id, quantity, created_at, updated_at FROM inventory WHERE product_id = ? AND warehouse_id = ?`
	var inv models.Inventory
	err := s.db.QueryRowContext(ctx, query, productID, warehouseID).Scan(&inv.ID, &inv.ProductID, &inv.WarehouseID, &inv.Quantity, &inv.CreatedAt, &inv.UpdatedAt)

	s.metrics.RecordDBQuery(ctx, "SELECT", "inventory", query, start, err == nil)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("inventory not found")
	}
	if err != nil {
		s.metrics.RecordDBQuery(ctx, "SELECT", "inventory", query, start, false)
		return nil, fmt.Errorf("failed to get inventory: %w", err)
	}

	// Update inventory gauge metric
	invAttrs := s.metrics.WithServiceName([]attribute.KeyValue{
		attribute.Int64("product_id", productID),
		attribute.String("warehouse_id", warehouseID),
	})
	log.Printf("[METRICS] Recording inventory level: product_id=%d, warehouse_id=%s, quantity=%d", productID, warehouseID, inv.Quantity)
	s.metrics.InventoryLevel.Record(ctx, int64(inv.Quantity), metric.WithAttributes(invAttrs...))
	log.Printf("[METRICS] InventoryLevel metric recorded")

	return &inv, nil
}
