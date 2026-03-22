package openapi

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

// dbInstance is a shared database connection pool.
var dbInstance *sql.DB

// GetDB returns the initialized database connection.
func GetDB() *sql.DB {
	return dbInstance
}

// InitDB initializes the MySQL connection and creates required tables.
func InitDB() error {
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")

	if dbHost == "" || dbPort == "" || dbName == "" || dbUser == "" || dbPassword == "" {
		return fmt.Errorf("missing one or more required DB environment variables")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		dbUser, dbPassword, dbHost, dbPort, dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("sql.Open failed: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("db.Ping failed: %w", err)
	}

	// Basic connection pooling
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)

	dbInstance = db

	log.Printf("Connected to MySQL at %s:%s database=%s", dbHost, dbPort, dbName)

	// Create tables if they do not exist
	if err := createTables(); err != nil {
		return err
	}

	// NOTE:
	// orders table is not used by current endpoints,
	// but is included for future extension (e.g., checkout flow).

	return nil
}

func seedProductsIfEmpty() error {
	var count int

	err := dbInstance.QueryRow("SELECT COUNT(*) FROM products").Scan(&count)
	if err != nil {
		return fmt.Errorf("count products failed: %w", err)
	}

	if count > 0 {
		log.Printf("Products table already seeded")
		return nil
	}

	_, err = dbInstance.Exec(`
		INSERT INTO products (product_id, sku, manufacturer, category_id, weight, some_other_id)
		VALUES
		(12345, 'ABC-123-XYZ', 'Acme Corporation', 456, 1250, 789),
		(12346, 'ABC-456-XYZ', 'Company B', 10, 15, 240),
		(12347, 'ABC-789-XYZ', 'Company A', 10, 20, 30)
	`)
	if err != nil {
		return fmt.Errorf("seed products failed: %w", err)
	}

	log.Printf("Seeded products")
	return nil
}

// createTables creates all required schema objects.
func createTables() error {
	queries := []string{
		// Product catalog
		`CREATE TABLE IF NOT EXISTS products (
			product_id INT PRIMARY KEY,
			sku VARCHAR(100) NOT NULL,
			manufacturer VARCHAR(200) NOT NULL,
			category_id INT NOT NULL,
			weight INT NOT NULL,
			some_other_id INT NOT NULL
		)`,

		// Shopping cart metadata
		`CREATE TABLE IF NOT EXISTS shopping_carts (
			shopping_cart_id INT PRIMARY KEY AUTO_INCREMENT,
			customer_id INT NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			checked_out_at DATETIME NULL,
			INDEX idx_shopping_carts_customer_id (customer_id),
			INDEX idx_shopping_carts_customer_updated (customer_id, updated_at)
		)`,

		// Cart items (many-to-one with carts)
		`CREATE TABLE IF NOT EXISTS shopping_cart_items (
			id INT PRIMARY KEY AUTO_INCREMENT,
			shopping_cart_id INT NOT NULL,
			product_id INT NOT NULL,
			quantity INT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			CONSTRAINT fk_cart_items_cart
				FOREIGN KEY (shopping_cart_id) REFERENCES shopping_carts(shopping_cart_id)
				ON DELETE CASCADE,
			CONSTRAINT fk_cart_items_product
				FOREIGN KEY (product_id) REFERENCES products(product_id),
			CONSTRAINT uq_cart_product UNIQUE (shopping_cart_id, product_id),
			INDEX idx_cart_items_cart_id (shopping_cart_id),
			INDEX idx_cart_items_product_id (product_id)
		)`,

		// Orders table (currently unused by API, reserved for checkout extension)
		`CREATE TABLE IF NOT EXISTS orders (
			order_id INT PRIMARY KEY AUTO_INCREMENT,
			shopping_cart_id INT NOT NULL,
			customer_id INT NOT NULL,
			created_at DATETIME NOT NULL,
			CONSTRAINT fk_orders_cart
				FOREIGN KEY (shopping_cart_id) REFERENCES shopping_carts(shopping_cart_id),
			CONSTRAINT uq_orders_cart UNIQUE (shopping_cart_id),
			INDEX idx_orders_customer_id (customer_id)
		)`,
	}

	for _, q := range queries {
		_, err := dbInstance.Exec(q)
		if err != nil {
			return fmt.Errorf("create table failed: %w", err)
		}
	}

	log.Printf("All tables created or verified")
	return nil
}

