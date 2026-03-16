variable "queue_name" {
  description = "Name of the SQS queue"
  type        = string
}

variable "sns_topic_arn" {
  description = "ARN of the SNS topic allowed to send messages to this queue"
  type        = string
}