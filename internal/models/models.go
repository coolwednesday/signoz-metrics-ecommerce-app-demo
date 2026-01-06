package models

import "time"

// Product represents a product in the catalog
type Product struct {
	ID          int64     `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Price       float64   `json:"price" db:"price"`
	Category    string    `json:"category" db:"category"`
	SKU         string    `json:"sku" db:"sku"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// User represents a user account
type User struct {
	ID        int64     `json:"id" db:"id"`
	Email     string    `json:"email" db:"email"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Cart represents a shopping cart
type Cart struct {
	ID        int64     `json:"id" db:"id"`
	UserID    int64     `json:"user_id" db:"user_id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// CartItem represents an item in a cart
type CartItem struct {
	ID        int64     `json:"id" db:"id"`
	CartID    int64     `json:"cart_id" db:"cart_id"`
	ProductID int64     `json:"product_id" db:"product_id"`
	Quantity  int       `json:"quantity" db:"quantity"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Order represents an order
type Order struct {
	ID            int64     `json:"id" db:"id"`
	UserID        int64     `json:"user_id" db:"user_id"`
	Status        string    `json:"status" db:"status"` // pending, completed, cancelled
	PaymentMethod string    `json:"payment_method" db:"payment_method"`
	TotalAmount   float64   `json:"total_amount" db:"total_amount"`
	Currency      string    `json:"currency" db:"currency"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// OrderItem represents an item in an order
type OrderItem struct {
	ID        int64     `json:"id" db:"id"`
	OrderID   int64     `json:"order_id" db:"order_id"`
	ProductID int64     `json:"product_id" db:"product_id"`
	Quantity  int       `json:"quantity" db:"quantity"`
	Price     float64   `json:"price" db:"price"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Inventory represents product inventory
type Inventory struct {
	ID          int64     `json:"id" db:"id"`
	ProductID   int64     `json:"product_id" db:"product_id"`
	WarehouseID string    `json:"warehouse_id" db:"warehouse_id"`
	Quantity    int       `json:"quantity" db:"quantity"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// CartResponse represents a cart with its items
type CartResponse struct {
	Cart  *Cart      `json:"cart"`
	Items []CartItem `json:"items"`
	Total float64    `json:"total"`
}

// AddToCartRequest represents a request to add item to cart
type AddToCartRequest struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

// CreateOrderRequest represents a request to create an order
type CreateOrderRequest struct {
	PaymentMethod string `json:"payment_method"`
	Currency      string `json:"currency"`
}

// CreateUserRequest represents a request to create a user
type CreateUserRequest struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}
