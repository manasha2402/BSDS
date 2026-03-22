variable "service_name" {
  description = "Base name for network resources"
  type        = string
}

variable "container_port" {
  description = "Port exposed by the container"
  type        = number
}

variable "aws_region" {
  description = "AWS region"
  type        = string
}