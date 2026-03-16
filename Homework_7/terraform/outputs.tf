output "receiver_cluster_name" {
  description = "ECS cluster name for order receiver"
  value       = module.ecs_receiver.cluster_name
}

output "receiver_service_name" {
  description = "ECS service name for order receiver"
  value       = module.ecs_receiver.service_name
}

output "alb_dns_name" {
  description = "ALB DNS name - use this for load testing"
  value       = module.ecs_receiver.alb_dns_name
}

output "processor_cluster_name" {
  description = "ECS cluster name for order processor"
  value       = module.ecs_processor.cluster_name
}

output "processor_service_name" {
  description = "ECS service name for order processor"
  value       = module.ecs_processor.service_name
}

output "sns_topic_arn" {
  description = "ARN of the SNS topic for order processing events"
  value       = module.sns.topic_arn
}

output "sqs_queue_url" {
  description = "URL of the SQS queue for order processing"
  value       = module.sqs.queue_url
}