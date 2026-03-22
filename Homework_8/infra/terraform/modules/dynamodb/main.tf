resource "aws_dynamodb_table" "shopping_carts" {
  name         = "shopping_carts_dynamo"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "cart_id"

  attribute {
    name = "cart_id"
    type = "S"
  }

  tags = {
    Name = "shopping-carts-dynamo"
  }
}

output "table_name" {
  value = aws_dynamodb_table.shopping_carts.name
}

output "table_arn" {
  value = aws_dynamodb_table.shopping_carts.arn
}