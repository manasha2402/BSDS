# Region to deploy into
variable "aws_region" {
  type    = string
  default = "us-west-2"
}

# ECR & ECS settings
variable "ecr_repository_name" {
  type    = string
  default = "ecr_service"
}

variable "service_name" {
  type    = string
  default = "CS6650L2"
}

variable "container_port" {
  type    = number
  default = 8080
}

variable "ecs_count" {
  type    = number
  default = 2
}

# How long to keep logs
variable "log_retention_days" {
  type    = number
  default = 7
}

# Auto Scaling settings
variable "min_instances" {
  type    = number
  default = 2
}

variable "max_instances" {
  type    = number
  default = 4
}

variable "cpu_target" {
  type    = number
  default = 70
}

variable "scale_cooldown" {
  type    = number
  default = 300
}