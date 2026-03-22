output "vpc_id" {
  description = "Custom VPC ID"
  value       = aws_vpc.this.id
}

output "public_subnet_ids" {
  description = "Public subnet IDs for the ALB"
  value = [
    aws_subnet.public_a.id,
    aws_subnet.public_b.id
  ]
}

output "private_subnet_ids" {
  description = "Private subnet IDs for ECS"
  value = [
    aws_subnet.private_a.id,
    aws_subnet.private_b.id
  ]
}

output "security_group_id" {
  description = "Security group ID for ECS tasks"
  value       = aws_security_group.this.id
}