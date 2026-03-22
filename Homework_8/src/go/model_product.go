package openapi

// Product represents a product in the catalog.
type Product struct {
	ProductId    int32  `json:"product_id"`
	Sku          string `json:"sku"`
	Manufacturer string `json:"manufacturer"`
	CategoryId   int32  `json:"category_id"`
	Weight       int32  `json:"weight"`
	SomeOtherId  int32  `json:"some_other_id"`
}

// CreateShoppingCartRequest is the request body for creating a cart.
type CreateShoppingCartRequest struct {
	CustomerId int32 `json:"customer_id"`
}

// CreateShoppingCartResponse returns the created cart ID.
type CreateShoppingCartResponse struct {
	ShoppingCartId int32 `json:"shopping_cart_id"`
}

// AddItemsToCartRequest is the request body for adding items to a cart.
type AddItemsToCartRequest struct {
	ProductId int32 `json:"product_id"`
	Quantity  int32 `json:"quantity"`
}

// CartItem represents a single item in a shopping cart.
type CartItem struct {
	ProductId int32 `json:"product_id"`
	Quantity  int32 `json:"quantity"`
}

// GetShoppingCartResponse represents a full cart with items.
type GetShoppingCartResponse struct {
	ShoppingCartId int32     `json:"shopping_cart_id"`
	CustomerId     int32     `json:"customer_id"`
	Status         string    `json:"status"`
	Items          []CartItem `json:"items"`
}

// ErrorResponse is a standard error payload.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}
