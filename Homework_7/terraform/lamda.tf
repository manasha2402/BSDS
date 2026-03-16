# --- ECR Repository for Lambda image ---
resource "aws_ecr_repository" "lambda" {
  name         = "order-lambda"
  force_delete = true
}

# --- Build and push Lambda Docker image ---
resource "docker_image" "lambda" {
  name = "${aws_ecr_repository.lambda.repository_url}:latest"

  build {
    context    = "../src"
    dockerfile = "lambda/Dockerfile"
  }
}

resource "docker_registry_image" "lambda" {
  name = docker_image.lambda.name
}

# --- Lambda Function ---
resource "aws_lambda_function" "order_processor" {
  function_name = "order-lambda"
  role          = data.aws_iam_role.lab_role.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.lambda.repository_url}:latest"
  memory_size   = 512
  timeout       = 30  # must be > 3s payment processing time

  depends_on = [docker_registry_image.lambda]
}

# --- SNS Trigger ---
# Allow SNS to invoke the Lambda function
resource "aws_lambda_permission" "sns" {
  statement_id  = "AllowSNSInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.order_processor.function_name
  principal     = "sns.amazonaws.com"
  source_arn    = module.sns.topic_arn
}

# Subscribe Lambda directly to the SNS topic — no SQS needed
resource "aws_sns_topic_subscription" "lambda" {
  topic_arn = module.sns.topic_arn
  protocol  = "lambda"
  endpoint  = aws_lambda_function.order_processor.arn
}

# --- Output ---
output "lambda_function_name" {
  value = aws_lambda_function.order_processor.function_name
}