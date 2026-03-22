package openapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Route stores the information for each API route.
type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc gin.HandlerFunc
}

// NewRouter creates a new Gin router with all routes registered.
func NewRouter(handleFunctions ApiHandleFunctions) *gin.Engine {
	return NewRouterWithGinEngine(gin.Default(), handleFunctions)
}

// NewRouterWithGinEngine adds routes to an existing Gin engine.
func NewRouterWithGinEngine(router *gin.Engine, handleFunctions ApiHandleFunctions) *gin.Engine {
	for _, route := range getRoutes(handleFunctions) {
		if route.HandlerFunc == nil {
			route.HandlerFunc = DefaultHandleFunc
		}

		switch route.Method {
		case http.MethodGet:
			router.GET(route.Pattern, route.HandlerFunc)
		case http.MethodPost:
			router.POST(route.Pattern, route.HandlerFunc)
		case http.MethodPut:
			router.PUT(route.Pattern, route.HandlerFunc)
		case http.MethodPatch:
			router.PATCH(route.Pattern, route.HandlerFunc)
		case http.MethodDelete:
			router.DELETE(route.Pattern, route.HandlerFunc)
		}
	}

	return router
}

// DefaultHandleFunc is used for routes that are not yet implemented.
func DefaultHandleFunc(c *gin.Context) {
	c.String(http.StatusNotImplemented, "501 not implemented")
}

// HealthHandleFunc provides a simple health check for the load balancer.
func HealthHandleFunc(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

// ApiHandleFunctions groups handler structs used by the router.
type ApiHandleFunctions struct {
	ShoppingCartAPI       ShoppingCartAPI
	DynamoShoppingCartAPI DynamoShoppingCartAPI // for DynamoDB-based shopping cart handlers
}

// getRoutes returns the routes required for this homework.
func getRoutes(handleFunctions ApiHandleFunctions) []Route {
	return []Route{
		{
			"Health",
			http.MethodGet,
			"/health",
			HealthHandleFunc,
		},
		{
			"CreateShoppingCart",
			http.MethodPost,
			"/shopping-carts",
			handleFunctions.ShoppingCartAPI.CreateShoppingCart,
		},
		{
			"GetShoppingCart",
			http.MethodGet,
			"/shopping-carts/:shoppingCartId",
			handleFunctions.ShoppingCartAPI.GetShoppingCart,
		},
		{
			"AddItemsToCart",
			http.MethodPost,
			"/shopping-carts/:shoppingCartId/items",
			handleFunctions.ShoppingCartAPI.AddItemsToCart,
		},

		{
			"DynamoCreateShoppingCart",
			http.MethodPost,
			"/dynamo/shopping-carts",
			handleFunctions.DynamoShoppingCartAPI.CreateShoppingCart,
		},
		{
			"DynamoGetShoppingCart",
			http.MethodGet,
			"/dynamo/shopping-carts/:shoppingCartId",
			handleFunctions.DynamoShoppingCartAPI.GetShoppingCart,
		},
		{
			"DynamoAddItemsToCart",
			http.MethodPost,
			"/dynamo/shopping-carts/:shoppingCartId/items",
			handleFunctions.DynamoShoppingCartAPI.AddItemsToCart,
		},
	}

}
