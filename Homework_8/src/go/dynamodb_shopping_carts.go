package openapi

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type DynamoShoppingCartAPI struct{}

func tableName() string {
	t := os.Getenv("DYNAMODB_TABLE")
	if t == "" {
		return "shopping_carts_dynamo"
	}
	return t
}

// POST /dynamo/shopping-carts
func (api *DynamoShoppingCartAPI) CreateShoppingCart(c *gin.Context) {
	client := GetDynamoDB()
	if client == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "DynamoDB not initialized",
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
	if req.CustomerId < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid input data",
			Details: "customer_id must be a positive integer",
		})
		return
	}

	cartId := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := client.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(tableName()),
		Item: map[string]types.AttributeValue{
			"cart_id":     &types.AttributeValueMemberS{Value: cartId},
			"customer_id": &types.AttributeValueMemberN{Value: strconv.Itoa(int(req.CustomerId))},
			"status":      &types.AttributeValueMemberS{Value: "active"},
			"items":       &types.AttributeValueMemberL{Value: []types.AttributeValue{}},
			"created_at":  &types.AttributeValueMemberS{Value: now},
			"updated_at":  &types.AttributeValueMemberS{Value: now},
		},
	})
	if err != nil {
		log.Printf("ERROR creating DynamoDB cart: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"shopping_cart_id": cartId})
}

// GET /dynamo/shopping-carts/:shoppingCartId
func (api *DynamoShoppingCartAPI) GetShoppingCart(c *gin.Context) {
	client := GetDynamoDB()
	if client == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "DynamoDB not initialized",
		})
		return
	}

	cartId := c.Param("shoppingCartId")
	if cartId == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid shoppingCartId",
		})
		return
	}

	result, err := client.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName: aws.String(tableName()),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberS{Value: cartId},
		},
	})
	if err != nil {
		log.Printf("ERROR getting DynamoDB cart: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}
	if result.Item == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "NOT_FOUND",
			Message: "Shopping cart not found",
		})
		return
	}

	customerId := 0
	if v, ok := result.Item["customer_id"].(*types.AttributeValueMemberN); ok {
		customerId, _ = strconv.Atoi(v.Value)
	}

	status := ""
	if v, ok := result.Item["status"].(*types.AttributeValueMemberS); ok {
		status = v.Value
	}

	var items []CartItem
	if v, ok := result.Item["items"].(*types.AttributeValueMemberL); ok {
		for _, iv := range v.Value {
			if m, ok := iv.(*types.AttributeValueMemberM); ok {
				var item CartItem
				if pid, ok := m.Value["product_id"].(*types.AttributeValueMemberN); ok {
					pidInt, _ := strconv.Atoi(pid.Value)
					item.ProductId = int32(pidInt)
				}
				if qty, ok := m.Value["quantity"].(*types.AttributeValueMemberN); ok {
					qtyInt, _ := strconv.Atoi(qty.Value)
					item.Quantity = int32(qtyInt)
				}
				items = append(items, item)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"shopping_cart_id": cartId,
		"customer_id":      customerId,
		"status":           status,
		"items":            items,
	})
}

// POST /dynamo/shopping-carts/:shoppingCartId/items
func (api *DynamoShoppingCartAPI) AddItemsToCart(c *gin.Context) {
	client := GetDynamoDB()
	if client == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "DynamoDB not initialized",
		})
		return
	}

	cartId := c.Param("shoppingCartId")
	if cartId == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid shoppingCartId",
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
	if req.ProductId < 1 || req.Quantity < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid input data",
			Details: "product_id and quantity must be positive integers",
		})
		return
	}

	// Fetch cart to verify it exists and is active
	result, err := client.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName: aws.String(tableName()),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberS{Value: cartId},
		},
	})
	if err != nil {
		log.Printf("ERROR getting DynamoDB cart for update: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}
	if result.Item == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "NOT_FOUND",
			Message: "Shopping cart not found",
		})
		return
	}

	status := ""
	if v, ok := result.Item["status"].(*types.AttributeValueMemberS); ok {
		status = v.Value
	}
	if status != "active" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid shopping cart state",
			Details: "cart is not active",
		})
		return
	}

	// Get existing items list
	var items []types.AttributeValue
	if v, ok := result.Item["items"].(*types.AttributeValueMemberL); ok {
		items = v.Value
	}

	// Update quantity if product already in cart, else append
	productIdStr := strconv.Itoa(int(req.ProductId))
	found := false
	for i, iv := range items {
		if m, ok := iv.(*types.AttributeValueMemberM); ok {
			if pid, ok := m.Value["product_id"].(*types.AttributeValueMemberN); ok && pid.Value == productIdStr {
				existingQty := 0
				if qty, ok := m.Value["quantity"].(*types.AttributeValueMemberN); ok {
					existingQty, _ = strconv.Atoi(qty.Value)
				}
				items[i] = &types.AttributeValueMemberM{
					Value: map[string]types.AttributeValue{
						"product_id": &types.AttributeValueMemberN{Value: productIdStr},
						"quantity":   &types.AttributeValueMemberN{Value: strconv.Itoa(existingQty + int(req.Quantity))},
					},
				}
				found = true
				break
			}
		}
	}
	if !found {
		items = append(items, &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				"product_id": &types.AttributeValueMemberN{Value: productIdStr},
				"quantity":   &types.AttributeValueMemberN{Value: strconv.Itoa(int(req.Quantity))},
			},
		})
	}

	now := time.Now().UTC().Format(time.RFC3339)

	_, err = client.UpdateItem(context.TODO(), &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName()),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberS{Value: cartId},
		},
		UpdateExpression: aws.String("SET #itms = :items, updated_at = :updated_at"),
		ExpressionAttributeNames: map[string]string{
			"#itms": "items",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":items":      &types.AttributeValueMemberL{Value: items},
			":updated_at": &types.AttributeValueMemberS{Value: now},
		},
	})
	if err != nil {
		log.Printf("ERROR updating DynamoDB cart items: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
		return
	}

	c.Status(http.StatusNoContent)
}
