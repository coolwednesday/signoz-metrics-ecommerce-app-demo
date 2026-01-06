package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/SigNoz/ecommerce-go-app/internal/db"
	"github.com/SigNoz/ecommerce-go-app/internal/metrics"
	"github.com/SigNoz/ecommerce-go-app/internal/middleware"
	"github.com/SigNoz/ecommerce-go-app/internal/models"
	"github.com/SigNoz/ecommerce-go-app/internal/services"
	"github.com/SigNoz/ecommerce-go-app/pkg/config"
	"github.com/gorilla/mux"
)

// App holds application dependencies
type App struct {
	config         *config.Config
	db             *db.DB
	metrics        *metrics.AppMetrics
	productService *services.ProductService
	cartService    *services.CartService
	orderService   *services.OrderService
	userService    *services.UserService
}

// NewApp creates a new application instance
func NewApp(
	cfg *config.Config,
	database *db.DB,
	m *metrics.AppMetrics,
	ps *services.ProductService,
	cs *services.CartService,
	os *services.OrderService,
	us *services.UserService,
) *App {
	return &App{
		config:         cfg,
		db:             database,
		metrics:        m,
		productService: ps,
		cartService:    cs,
		orderService:   os,
		userService:    us,
	}
}

// SetupRoutes configures the HTTP routes
func (a *App) SetupRoutes(r *mux.Router) {
	// Middleware
	r.Use(middleware.RequestIDMiddleware)
	r.Use(middleware.CORSMiddleware)
	r.Use(middleware.ErrorHandlerMiddleware)
	r.Use(middleware.MetricsMiddleware(a.metrics))

	// API Routes
	api := r.PathPrefix("/api/v1").Subrouter()

	// Products
	api.HandleFunc("/products", a.ListProductsHandler).Methods("GET")
	api.HandleFunc("/products/{id}", a.GetProductHandler).Methods("GET")
	api.HandleFunc("/products/{id}/inventory", a.GetProductInventoryHandler).Methods("GET")

	// Cart
	api.HandleFunc("/cart", a.GetCartHandler).Methods("GET")
	api.HandleFunc("/cart/add", a.AddToCartHandler).Methods("POST")
	api.HandleFunc("/cart/remove", a.RemoveFromCartHandler).Methods("POST")

	// Orders
	api.HandleFunc("/orders", a.CreateOrderHandler).Methods("POST")
	api.HandleFunc("/orders", a.ListOrdersHandler).Methods("GET")
	api.HandleFunc("/orders/{id}", a.GetOrderHandler).Methods("GET")
	api.HandleFunc("/orders/{id}/status", a.UpdateOrderStatusHandler).Methods("PUT")

	// Users
	api.HandleFunc("/users", a.CreateUserHandler).Methods("POST")
	api.HandleFunc("/users/{id}", a.GetUserHandler).Methods("GET")

	// Health
	r.HandleFunc("/health", a.HealthHandler).Methods("GET")
}

// HealthHandler handles health check requests
func (a *App) HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// ListProductsHandler handles GET /api/v1/products
func (a *App) ListProductsHandler(w http.ResponseWriter, r *http.Request) {
	limit := 20
	offset := 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil {
			offset = parsed
		}
	}

	products, err := a.productService.ListProducts(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(products)
}

// GetProductHandler handles GET /api/v1/products/{id}
func (a *App) GetProductHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid product ID", http.StatusBadRequest)
		return
	}

	product, err := a.productService.GetProduct(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(product)
}

// GetProductInventoryHandler handles GET /api/v1/products/{id}/inventory
func (a *App) GetProductInventoryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid product ID", http.StatusBadRequest)
		return
	}

	warehouseID := r.URL.Query().Get("warehouse_id")
	if warehouseID == "" {
		warehouseID = "WH-001" // Default warehouse
	}

	inventory, err := a.productService.GetProductInventory(r.Context(), id, warehouseID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inventory)
}

// AddToCartHandler handles POST /api/v1/cart/add
func (a *App) AddToCartHandler(w http.ResponseWriter, r *http.Request) {
	var req models.AddToCartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// For simplicity, use user_id from query param or default to 1
	userID := int64(1)
	if uid := r.URL.Query().Get("user_id"); uid != "" {
		if parsed, err := strconv.ParseInt(uid, 10, 64); err == nil {
			userID = parsed
		}
	}

	if err := a.cartService.AddToCart(r.Context(), userID, req.ProductID, req.Quantity); err != nil {
		if err.Error() == "product not found" {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "added"})
}

// RemoveFromCartHandler handles POST /api/v1/cart/remove
func (a *App) RemoveFromCartHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProductID int64 `json:"product_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	userID := int64(1)
	if uid := r.URL.Query().Get("user_id"); uid != "" {
		if parsed, err := strconv.ParseInt(uid, 10, 64); err == nil {
			userID = parsed
		}
	}

	if err := a.cartService.RemoveFromCart(r.Context(), userID, req.ProductID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
}

// GetCartHandler handles GET /api/v1/cart
func (a *App) GetCartHandler(w http.ResponseWriter, r *http.Request) {
	userID := int64(1)
	if uid := r.URL.Query().Get("user_id"); uid != "" {
		if parsed, err := strconv.ParseInt(uid, 10, 64); err == nil {
			userID = parsed
		}
	}

	cart, err := a.cartService.GetCart(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cart)
}

// CreateOrderHandler handles POST /api/v1/orders
func (a *App) CreateOrderHandler(w http.ResponseWriter, r *http.Request) {
	var req models.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Currency == "" {
		req.Currency = "USD"
	}
	if req.PaymentMethod == "" {
		req.PaymentMethod = "credit_card"
	}

	userID := int64(1)
	if uid := r.URL.Query().Get("user_id"); uid != "" {
		if parsed, err := strconv.ParseInt(uid, 10, 64); err == nil {
			userID = parsed
		}
	}

	order, err := a.orderService.CreateOrder(r.Context(), userID, req.PaymentMethod, req.Currency)
	if err != nil {
		if err.Error() == "cart is empty" {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(order)
}

// GetOrderHandler handles GET /api/v1/orders/{id}
func (a *App) GetOrderHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid order ID", http.StatusBadRequest)
		return
	}

	order, err := a.orderService.GetOrder(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

// ListOrdersHandler handles GET /api/v1/orders
func (a *App) ListOrdersHandler(w http.ResponseWriter, r *http.Request) {
	userID := int64(1)
	if uid := r.URL.Query().Get("user_id"); uid != "" {
		if parsed, err := strconv.ParseInt(uid, 10, 64); err == nil {
			userID = parsed
		}
	}

	orders, err := a.orderService.ListUserOrders(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}

// CreateUserHandler handles POST /api/v1/users
func (a *App) CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	var req models.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := a.userService.CreateUser(r.Context(), req.ID, req.Email, req.Name)
	if err != nil {
		if err.Error() == "user already exists" {
			// Find the existing user to return their ID
			// For now, we'll just return the error message,
			// but the traffic script needs the ID.
			// I'll add a GetUserByEmail method to UserService.
			existingUser, lookupErr := a.userService.GetUserByEmail(r.Context(), req.Email)
			if lookupErr == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(existingUser)
				return
			}
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

// GetUserHandler handles GET /api/v1/users/{id}
func (a *App) GetUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	user, err := a.userService.GetUser(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// UpdateOrderStatusHandler handles PUT /api/v1/orders/{id}/status
func (a *App) UpdateOrderStatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orderID, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid order ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := a.orderService.UpdateOrderStatus(r.Context(), orderID, req.Status); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}
