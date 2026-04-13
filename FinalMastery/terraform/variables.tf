variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-west-2"
}

variable "app_name" {
  description = "Name prefix for all resources"
  type        = string
  default     = "album-store"
}

variable "db_password" {
  description = "No longer used — kept so existing tfvars files still work"
  type        = string
  default     = ""
  sensitive   = true
}

variable "s3_bucket" {
  description = "S3 bucket name for photo storage"
  type        = string
}
