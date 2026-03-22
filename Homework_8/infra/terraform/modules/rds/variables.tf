variable "db_name" {
  type        = string
  description = "Initial database name"
  default     = "shoppingcart"
}

variable "db_username" {
  type        = string
  description = "Master username for RDS"
  default     = "admin"
}

variable "db_password" {
  type        = string
  description = "Master password for RDS"
  sensitive   = true
}

variable "subnet_ids" {
  type        = list(string)
  description = "Subnet IDs for the DB subnet group"
}

variable "vpc_id" {
  type        = string
  description = "VPC ID where RDS will be created"
}

variable "ecs_security_group_id" {
  type        = string
  description = "ECS security group allowed to connect to MySQL"
}

variable "db_port" {
  type        = number
  description = "MySQL port"
  default     = 3306
}

variable "db_instance_class" {
  type        = string
  description = "RDS instance class"
  default     = "db.t3.micro"
}

variable "allocated_storage" {
  type        = number
  description = "Allocated storage in GB"
  default     = 20
}
