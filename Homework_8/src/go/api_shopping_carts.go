package openapi

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// ShoppingCartAPI handles shopping cart endpoints.
type ShoppingCartAPI struct{}

// POST /shopping-carts
// Creates a new shopping cart for a customer.
func (api *ShoppingCartAPI) CreateShoppingCart(c *gin.Context) {
	db := GetDB()
	if db == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Database not initialized",
		})
		return
	}

	var req CreateShoppingCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid input data",
			Details: err.Error(),
		})
		return
	}

	// Validate customer id
	if req.CustomerId < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid input data",
			Details: "customer_id must be a positive integer",
		})
		return
	}

	now := time.Now()

	// Insert new cart
	result, err := db.Exec(`
		INSERT INTO shopping_carts (customer_id, status, created_at, updated_at, checked_out_at)
		VALUES (?, 'active', ?, ?, NULL)
	`, req.CustomerId, now, now)
	if err != nil {
		log.Printf("ERROR create shopping cart failed: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}

	lastId, err := result.LastInsertId()
	if err != nil {
		log.Printf("ERROR reading shopping cart id failed: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}

	c.JSON(http.StatusCreated, CreateShoppingCartResponse{
		ShoppingCartId: int32(lastId),
	})
}

// GET /shopping-carts/:shoppingCartId
// Retrieves a shopping cart and all its items.
func (api *ShoppingCartAPI) GetShoppingCart(c *gin.Context) {
	db := GetDB()
	if db == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Database not initialized",
		})
		return
	}

	cartId, err := strconv.Atoi(c.Param("shoppingCartId"))
	if err != nil || cartId < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid shoppingCartId",
		})
		return
	}

	// Query cart metadata
	var customerId int
	var status string

	err = db.QueryRow(`
		SELECT customer_id, status
		FROM shopping_carts
		WHERE shopping_cart_id = ?
	`, cartId).Scan(&customerId, &status)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "NOT_FOUND",
			Message: "Shopping cart not found",
		})
		return
	}
	if err != nil {
		log.Printf("ERROR reading cart: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}

	// Query items using JOIN
	rows, err := db.Query(`
		SELECT product_id, quantity
		FROM shopping_cart_items
		WHERE shopping_cart_id = ?
	`, cartId)
	if err != nil {
		log.Printf("ERROR reading cart items: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}
	defer rows.Close()

	var items []CartItem
	for rows.Next() {
		var item CartItem
		if err := rows.Scan(&item.ProductId, &item.Quantity); err != nil {
			log.Printf("ERROR scanning cart item: %v", err)
			continue
		}
		items = append(items, item)
	}

	// Return full cart
	c.JSON(http.StatusOK, GetShoppingCartResponse{
		ShoppingCartId: int32(cartId),
		CustomerId:     int32(customerId),
		Status:         status,
		Items:          items,
	})
}

// POST /shopping-carts/:shoppingCartId/items
// Adds or updates items in an existing cart.
func (api *ShoppingCartAPI) AddItemsToCart(c *gin.Context) {
	db := GetDB()
	if db == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Database not initialized",
		})
		return
	}

	cartId, err := strconv.Atoi(c.Param("shoppingCartId"))
	if err != nil || cartId < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid input data",
			Details: "shoppingCartId must be a positive integer",
		})
		return
	}

	var req AddItemsToCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid input data",
			Details: err.Error(),
		})
		return
	}

	// Validate inputs
	if req.ProductId < 1 || req.Quantity < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid input data",
			Details: "product_id and quantity must be positive integers",
		})
		return
	}

	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}
	defer tx.Rollback()

	// Ensure cart exists and is active
	var status string
	err = tx.QueryRow(`
		SELECT status
		FROM shopping_carts
		WHERE shopping_cart_id = ?
	`, cartId).Scan(&status)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "NOT_FOUND",
			Message: "Shopping cart not found",
		})
		return
	}
	if err != nil {
		log.Printf("ERROR reading shopping cart failed: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}

	if status != "active" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid shopping cart state",
			Details: "cart is not active",
		})
		return
	}

	// Ensure product exists
	var productExists int
	err = tx.QueryRow(`
		SELECT 1 FROM products WHERE product_id = ?
	`, req.ProductId).Scan(&productExists)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "NOT_FOUND",
			Message: "Product not found",
		})
		return
	}
	if err != nil {
		log.Printf("ERROR reading product failed: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}

	now := time.Now()

	// Insert or update quantity
	_, err = tx.Exec(`
		INSERT INTO shopping_cart_items (shopping_cart_id, product_id, quantity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			quantity = quantity + VALUES(quantity),
			updated_at = VALUES(updated_at)
	`, cartId, req.ProductId, req.Quantity, now, now)
	if err != nil {
		log.Printf("ERROR inserting shopping cart item failed: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}

	// Update cart timestamp
	_, err = tx.Exec(`
		UPDATE shopping_carts
		SET updated_at = ?
		WHERE shopping_cart_id = ?
	`, now, cartId)
	if err != nil {
		log.Printf("ERROR updating shopping cart timestamp failed: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("ERROR committing add items transaction failed: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}

	c.Status(http.StatusNoContent)
}
