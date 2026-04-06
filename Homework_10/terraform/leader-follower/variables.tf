variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "service_name" {
  type    = string
  default = "kvstore"
}

variable "container_port" {
  type    = number
  default = 8080
}

variable "log_retention_days" {
  type    = number
  default = 7
}

# W and R quorum values — change these to switch strategies:
#   Strategy 1: W=5, R=1
#   Strategy 2: W=1, R=5
#   Strategy 3: W=3, R=3
variable "follower_peers" {
  type        = string
  default     = ""
  description = "Peers for followers — includes leader + all other followers"
}

variable "peers" {
  type        = string
  default     = ""
  description = "Comma-separated list of follower URLs. Set after first deploy."
}

variable "W" {
  type    = number
  default = 5
}

variable "R" {
  type    = number
  default = 1
}