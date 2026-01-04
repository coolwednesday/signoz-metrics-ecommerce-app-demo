-- E-Commerce Database Schema
-- Note: Database is created automatically by MySQL container via MYSQL_DATABASE env var
-- This file is executed in the context of the ecommerce database

-- Products table
CREATE TABLE IF NOT EXISTS products (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    price DECIMAL(10, 2) NOT NULL,
    category VARCHAR(100),
    sku VARCHAR(100) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_category (category),
    INDEX idx_sku (sku)
);

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_email (email)
);

-- Carts table
CREATE TABLE IF NOT EXISTS carts (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_user_id (user_id)
);

-- Cart items table
CREATE TABLE IF NOT EXISTS cart_items (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    cart_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    quantity INT NOT NULL DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (cart_id) REFERENCES carts(id) ON DELETE CASCADE,
    FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
    INDEX idx_cart_id (cart_id),
    INDEX idx_product_id (product_id)
);

-- Orders table
CREATE TABLE IF NOT EXISTS orders (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    payment_method VARCHAR(50),
    total_amount DECIMAL(10, 2) NOT NULL,
    currency VARCHAR(10) DEFAULT 'USD',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_user_id (user_id),
    INDEX idx_status (status),
    INDEX idx_created_at (created_at)
);

-- Order items table
CREATE TABLE IF NOT EXISTS order_items (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    order_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    quantity INT NOT NULL,
    price DECIMAL(10, 2) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE,
    FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
    INDEX idx_order_id (order_id),
    INDEX idx_product_id (product_id)
);

-- Inventory table
CREATE TABLE IF NOT EXISTS inventory (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    product_id BIGINT NOT NULL,
    warehouse_id VARCHAR(100) NOT NULL,
    quantity INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
    UNIQUE KEY unique_product_warehouse (product_id, warehouse_id),
    INDEX idx_product_id (product_id),
    INDEX idx_warehouse_id (warehouse_id)
);

-- Insert diverse sample data with multiple categories
INSERT INTO products (name, description, price, category, sku) VALUES
-- Electronics (7 products)
('Laptop', 'High-performance laptop', 999.99, 'Electronics', 'LAP-001'),
('Mouse', 'Wireless mouse', 29.99, 'Electronics', 'MOU-001'),
('Keyboard', 'Mechanical keyboard', 79.99, 'Electronics', 'KEY-001'),
('Monitor', '27-inch 4K monitor', 399.99, 'Electronics', 'MON-001'),
('Headphones', 'Noise-cancelling headphones', 199.99, 'Electronics', 'HEA-001'),
('Tablet', '10-inch tablet', 299.99, 'Electronics', 'TAB-001'),
('Smartphone', 'Latest smartphone', 699.99, 'Electronics', 'PHN-001'),
-- Clothing (5 products)
('T-Shirt', 'Cotton t-shirt', 19.99, 'Clothing', 'TSH-001'),
('Jeans', 'Classic blue jeans', 49.99, 'Clothing', 'JEA-001'),
('Sneakers', 'Running sneakers', 79.99, 'Clothing', 'SNK-001'),
('Jacket', 'Winter jacket', 89.99, 'Clothing', 'JCK-001'),
('Hat', 'Baseball cap', 14.99, 'Clothing', 'HAT-001'),
-- Books (3 products)
('Programming Book', 'Learn Go programming', 39.99, 'Books', 'BOK-001'),
('Novel', 'Bestselling novel', 12.99, 'Books', 'BOK-002'),
('Cookbook', 'Italian recipes', 24.99, 'Books', 'BOK-003'),
-- Home & Garden (3 products)
('Coffee Maker', 'Drip coffee maker', 59.99, 'Home & Garden', 'HOM-001'),
('Lamp', 'Desk lamp', 34.99, 'Home & Garden', 'HOM-002'),
('Plant Pot', 'Ceramic plant pot', 19.99, 'Home & Garden', 'HOM-003'),
-- Sports (4 products)
('Basketball', 'Official size basketball', 24.99, 'Sports', 'SPT-001'),
('Yoga Mat', 'Premium yoga mat', 29.99, 'Sports', 'SPT-002'),
('Dumbbells', '10lb dumbbells set', 49.99, 'Sports', 'SPT-003'),
('Tennis Racket', 'Professional tennis racket', 89.99, 'Sports', 'SPT-004')
ON DUPLICATE KEY UPDATE name=name;

-- Insert inventory for all products across multiple warehouses
INSERT INTO inventory (product_id, warehouse_id, quantity) VALUES
-- WH-001 (23 products)
(1, 'WH-001', 50), (2, 'WH-001', 200), (3, 'WH-001', 100), (4, 'WH-001', 30), (5, 'WH-001', 75),
(6, 'WH-001', 40), (7, 'WH-001', 60), (8, 'WH-001', 150), (9, 'WH-001', 80), (10, 'WH-001', 90),
(11, 'WH-001', 45), (12, 'WH-001', 25), (13, 'WH-001', 200), (14, 'WH-001', 180), (15, 'WH-001', 120),
(16, 'WH-001', 70), (17, 'WH-001', 55), (18, 'WH-001', 35), (19, 'WH-001', 100), (20, 'WH-001', 65),
(21, 'WH-001', 85), (22, 'WH-001', 95), (23, 'WH-001', 50),
-- WH-002 (23 products)
(1, 'WH-002', 30), (2, 'WH-002', 150), (3, 'WH-002', 60), (4, 'WH-002', 20), (5, 'WH-002', 50),
(6, 'WH-002', 25), (7, 'WH-002', 40), (8, 'WH-002', 100), (9, 'WH-002', 50), (10, 'WH-002', 60),
(11, 'WH-002', 30), (12, 'WH-002', 15), (13, 'WH-002', 150), (14, 'WH-002', 120), (15, 'WH-002', 80),
(16, 'WH-002', 45), (17, 'WH-002', 35), (18, 'WH-002', 25), (19, 'WH-002', 70), (20, 'WH-002', 40),
(21, 'WH-002', 55), (22, 'WH-002', 65), (23, 'WH-002', 30),
-- WH-003 (23 products)
(1, 'WH-003', 20), (2, 'WH-003', 100), (3, 'WH-003', 40), (4, 'WH-003', 15), (5, 'WH-003', 35),
(6, 'WH-003', 20), (7, 'WH-003', 30), (8, 'WH-003', 80), (9, 'WH-003', 40), (10, 'WH-003', 50),
(11, 'WH-003', 25), (12, 'WH-003', 10), (13, 'WH-003', 100), (14, 'WH-003', 90), (15, 'WH-003', 60),
(16, 'WH-003', 30), (17, 'WH-003', 25), (18, 'WH-003', 20), (19, 'WH-003', 50), (20, 'WH-003', 30),
(21, 'WH-003', 40), (22, 'WH-003', 45), (23, 'WH-003', 25)
ON DUPLICATE KEY UPDATE quantity=quantity;

