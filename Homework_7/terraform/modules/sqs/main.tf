resource "aws_sqs_queue" "this" {
  name                       = var.queue_name
  visibility_timeout_seconds = 30       # default - message hidden from others while being processed
  message_retention_seconds  = 345600   # 4 days
  receive_wait_time_seconds  = 20       # long polling - waits up to 20s for messages
}

# Allow SNS to send messages to this SQS queue
resource "aws_sqs_queue_policy" "this" {
  queue_url = aws_sqs_queue.this.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "sns.amazonaws.com" }
      Action    = "sqs:SendMessage"
      Resource  = aws_sqs_queue.this.arn
      Condition = {
        ArnEquals = {
          "aws:SourceArn" = var.sns_topic_arn
        }
      }
    }]
  })
}