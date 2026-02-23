output "subnet_ids" {
  description = "IDs of the default VPC subnets"
  value       = data.aws_subnets.default.ids
}

output "security_group_id" {
  description = "Security group ID for ECS"
  value       = aws_security_group.this.id
}
output "alb_security_group_id" {
  description = "Security group ID for ALB"
  value       = aws_security_group.alb.id
}

output "vpc_id" {
  description = "Default VPC ID"
  value       = data.aws_vpc.default.id
}
