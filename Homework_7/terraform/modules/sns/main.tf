resource "aws_sns_topic" "this" {
  name = var.topic_name
}

# Subscribe the SQS queue to this SNS topic
resource "aws_sns_topic_subscription" "sqs" {
  topic_arn = aws_sns_topic.this.arn
  protocol  = "sqs"
  endpoint  = var.sqs_queue_arn
}