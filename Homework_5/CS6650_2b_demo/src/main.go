package main

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
)

// Product matches the OpenAPI spec schema
type Product struct {
	ProductID    int    `json:"product_id"`
	SKU          string `json:"sku"`
	Manufacturer string `json:"manufacturer"`
	CategoryID   int    `json:"category_id"`
	Weight       int    `json:"weight"`
	SomeOtherID  int    `json:"some_other_id"`
}

// ErrorResponse matches the OpenAPI spec Error schema
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// In-memory storage: hashmap for O(1) lookups by product_id
var (
	products = make(map[int]Product)
	mu       sync.RWMutex // protects concurrent access to the map
)

func main() {
	r := gin.Default()

	// Product endpoints from OpenAPI spec
	r.GET("/products/:productId", getProduct)
	r.POST("/products/:productId/details", addProductDetails)

	// Additional: create products (needed to populate the store)
	r.POST("/products", createProduct)
	r.GET("/products", listProducts)

	r.Run(":8080")
}

// parseProductID extracts and validates the productId path parameter
func parseProductID(c *gin.Context) (int, bool) {
	idStr := c.Param("productId")
	id, err := strconv.Atoi(idStr)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Product ID must be a positive integer",
			Details: "productId path parameter must be an integer >= 1",
		})
		return 0, false
	}
	return id, true
}

// validateProduct checks all required fields per the OpenAPI spec
func validateProduct(p Product) *ErrorResponse {
	if p.ProductID < 1 {
		return &ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "product_id must be a positive integer",
		}
	}
	if p.SKU == "" || len(p.SKU) > 100 {
		return &ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "sku is required and must be 1-100 characters",
		}
	}
	if p.Manufacturer == "" || len(p.Manufacturer) > 200 {
		return &ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "manufacturer is required and must be 1-200 characters",
		}
	}
	if p.CategoryID < 1 {
		return &ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "category_id must be a positive integer",
		}
	}
	if p.Weight < 0 {
		return &ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "weight must be a non-negative integer",
		}
	}
	if p.SomeOtherID < 1 {
		return &ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "some_other_id must be a positive integer",
		}
	}
	return nil
}

// GET /products/:productId
// Returns 200 with product, 404 if not found
func getProduct(c *gin.Context) {
	id, ok := parseProductID(c)
	if !ok {
		return
	}

	mu.RLock()
	product, exists := products[id]
	mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "NOT_FOUND",
			Message: "Product not found",
			Details: "No product with ID " + strconv.Itoa(id),
		})
		return
	}

	c.JSON(http.StatusOK, product)
}

// GET /products
// Returns all products
func listProducts(c *gin.Context) {
	mu.RLock()
	defer mu.RUnlock()

	all := make([]Product, 0, len(products))
	for _, p := range products {
		all = append(all, p)
	}
	c.JSON(http.StatusOK, all)
}

// POST /products/:productId/details
// Returns 204 on success, 400 for invalid input, 404 if product doesn't exist
func addProductDetails(c *gin.Context) {
	id, ok := parseProductID(c)
	if !ok {
		return
	}

	var p Product
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid request body",
			Details: err.Error(),
		})
		return
	}

	if errResp := validateProduct(p); errResp != nil {
		c.JSON(http.StatusBadRequest, *errResp)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if _, exists := products[id]; !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "NOT_FOUND",
			Message: "Product not found",
			Details: "No product with ID " + strconv.Itoa(id),
		})
		return
	}

	p.ProductID = id
	products[id] = p
	c.Status(http.StatusNoContent)
}

// POST /products
// Creates a new product. Returns 201, or 400 for invalid input / duplicate
func createProduct(c *gin.Context) {
	var p Product
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid request body",
			Details: err.Error(),
		})
		return
	}

	if errResp := validateProduct(p); errResp != nil {
		c.JSON(http.StatusBadRequest, *errResp)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if _, exists := products[p.ProductID]; exists {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "DUPLICATE",
			Message: "Product already exists",
			Details: "Product with ID " + strconv.Itoa(p.ProductID) + " already exists",
		})
		return
	}

	products[p.ProductID] = p
	c.JSON(http.StatusCreated, p)
}
