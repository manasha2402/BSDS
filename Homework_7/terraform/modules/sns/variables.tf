variable "topic_name" {
  description = "Name of the SNS topic"
  type        = string
}

variable "sqs_queue_arn" {
  description = "ARN of the SQS queue to subscribe to this topic"
  type        = string
}