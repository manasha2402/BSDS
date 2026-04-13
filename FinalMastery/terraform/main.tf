terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

# ─── Data sources ──────────────────────────────────────────────────────────────

data "aws_caller_identity" "current" {}

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

data "aws_route_tables" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

# ─── ECR ───────────────────────────────────────────────────────────────────────

resource "aws_ecr_repository" "app" {
  name                 = var.app_name
  image_tag_mutability = "MUTABLE"
  force_delete         = true
}

# ─── DynamoDB ──────────────────────────────────────────────────────────────────
# PAY_PER_REQUEST: auto-scales instantly under load test bursts.
# No connection pool — HTTP API, unlimited concurrent callers.

resource "aws_dynamodb_table" "albums" {
  name         = "${var.app_name}-albums"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "album_id"

  attribute {
    name = "album_id"
    type = "S"
  }

  tags = { Name = "${var.app_name}-albums" }
}

resource "aws_dynamodb_table" "photos" {
  name         = "${var.app_name}-photos"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "photo_id"

  attribute {
    name = "photo_id"
    type = "S"
  }

  tags = { Name = "${var.app_name}-photos" }
}

# ─── IAM ───────────────────────────────────────────────────────────────────────

data "aws_iam_role" "lab_role" {
  name = "LabRole"
}

# ─── CloudWatch ────────────────────────────────────────────────────────────────

resource "aws_cloudwatch_log_group" "app" {
  name              = "/ecs/${var.app_name}"
  retention_in_days = 7
}

# ─── ECS ───────────────────────────────────────────────────────────────────────

resource "aws_ecs_cluster" "main" {
  name = "${var.app_name}-cluster"
}

resource "aws_ecs_task_definition" "app" {
  family                   = var.app_name
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "4096" # 4 vCPU — headroom for 50 concurrent S3 goroutines
  memory                   = "8192" # 8 GB — handles large file buffers under concurrent load
  execution_role_arn       = data.aws_iam_role.lab_role.arn
  task_role_arn            = data.aws_iam_role.lab_role.arn

  container_definitions = jsonencode([
    {
      name      = var.app_name
      image     = "${aws_ecr_repository.app.repository_url}:latest"
      essential = true

      portMappings = [
        {
          containerPort = 8080
          protocol      = "tcp"
        }
      ]

      environment = [
        { name = "PORT",         value = "8080" },
        { name = "ALBUMS_TABLE", value = aws_dynamodb_table.albums.name },
        { name = "PHOTOS_TABLE", value = aws_dynamodb_table.photos.name },
        { name = "S3_BUCKET",    value = var.s3_bucket },
        { name = "AWS_REGION",   value = var.aws_region },
        { name = "WORKER_COUNT", value = "50" }
      ]

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.app.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "ecs"
        }
      }
    }
  ])
}

# ─── Security groups ───────────────────────────────────────────────────────────

resource "aws_security_group" "alb" {
  name        = "${var.app_name}-alb-sg"
  description = "Allow HTTP from anywhere"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "ecs" {
  name        = "${var.app_name}-ecs-sg"
  description = "Allow traffic from ALB"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# ─── ALB ───────────────────────────────────────────────────────────────────────

resource "aws_lb" "main" {
  name               = "${var.app_name}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = data.aws_subnets.default.ids

  # 5-minute idle timeout — prevents ALB from dropping large file upload
  # connections mid-stream. Default 60 s is too short for S15 payloads.
  idle_timeout = 300
}

resource "aws_lb_target_group" "app" {
  name        = "${var.app_name}-tg"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = data.aws_vpc.default.id
  target_type = "ip"

  health_check {
    path                = "/health"
    interval            = 15
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }
}

# ─── ECS Service ───────────────────────────────────────────────────────────────

resource "aws_ecs_service" "app" {
  name            = "${var.app_name}-service"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.app.arn
  desired_count   = 3
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = data.aws_subnets.default.ids
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.app.arn
    container_name   = var.app_name
    container_port   = 8080
  }

  depends_on = [aws_lb_listener.http]
}

# ─── S3 VPC Gateway Endpoint ───────────────────────────────────────────────────
# Routes ECS→S3 over AWS's private backbone instead of the internet gateway.
# Free, zero application config, ~1500 ms improvement on large S3 uploads.

resource "aws_vpc_endpoint" "s3" {
  vpc_id            = data.aws_vpc.default.id
  service_name      = "com.amazonaws.${var.aws_region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = data.aws_route_tables.default.ids
}

# ─── Outputs ───────────────────────────────────────────────────────────────────

output "alb_url" {
  value = "http://${aws_lb.main.dns_name}"
}

output "ecr_repository_url" {
  value = aws_ecr_repository.app.repository_url
}

output "albums_table" {
  value = aws_dynamodb_table.albums.name
}

output "photos_table" {
  value = aws_dynamodb_table.photos.name
}
