# Wire together modules: network, ecr, logging, ecs, rds.

module "network" {
  source         = "./modules/network"
  service_name   = var.service_name
  container_port = var.container_port
  aws_region     = var.aws_region
}

module "ecr" {
  source          = "./modules/ecr"
  repository_name = var.ecr_repository_name
}

module "logging" {
  source            = "./modules/logging"
  service_name      = var.service_name
  retention_in_days = var.log_retention_days
}

# Reuse an existing IAM role for ECS tasks
data "aws_iam_role" "lab_role" {
  name = "LabRole"
}

module "ecs" {
  source                = "./modules/ecs"
  service_name          = var.service_name
  image                 = "${module.ecr.repository_url}:latest"
  container_port        = var.container_port
  subnet_ids            = module.network.public_subnet_ids
  security_group_ids    = [module.network.security_group_id]
  execution_role_arn    = data.aws_iam_role.lab_role.arn
  task_role_arn         = data.aws_iam_role.lab_role.arn
  log_group_name        = module.logging.log_group_name
  ecs_count             = 2
  region                = var.aws_region
  target_group_arn      = aws_lb_target_group.app.arn
  alb_security_group_id = aws_security_group.alb_sg.id
  db_host               = module.rds.db_address
  db_port               = module.rds.db_port
  db_name               = module.rds.db_name
  db_user               = module.rds.db_username
  db_password           = var.db_password
  dynamodb_table        = module.dynamodb.table_name
}


// Build & push the Go app image into ECR
resource "docker_image" "app" {
  # Use the URL from the ecr module, and tag it "latest"
  name = "${module.ecr.repository_url}:latest"

  build {
    # relative path from terraform/ → src/
    context = "${path.module}/../../src"
    # Dockerfile defaults to "Dockerfile" in that context
  }
}

resource "docker_registry_image" "app" {
  # this will push :latest → ECR
  name = docker_image.app.name
}

# --- ALB Security Group (internet -> ALB on 80) ---
resource "aws_security_group" "alb_sg" {
  name   = "cs6650-alb-sg"
  vpc_id = module.network.vpc_id

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

# --- Application Load Balancer ---
resource "aws_lb" "app" {
  name               = "cs6650-alb"
  load_balancer_type = "application"
  subnets            = module.network.public_subnet_ids
  security_groups    = [aws_security_group.alb_sg.id]
}

# --- Target Group (Fargate requires target_type = ip) ---
resource "aws_lb_target_group" "app" {
  name        = "cs6650-tg"
  port        = 8080
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = module.network.vpc_id

  health_check {
    path                = "/health"
    interval            = 30
    healthy_threshold   = 2
    unhealthy_threshold = 2
    matcher             = "200"
  }
}

# --- Listener: :80 -> target group ---
resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.app.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }
}

# Allow the ALB to reach ECS tasks on the container port (8080)
resource "aws_security_group_rule" "ecs_ingress_from_alb" {
  type                     = "ingress"
  security_group_id        = module.network.security_group_id   # ECS task SG
  from_port                = var.container_port
  to_port                  = var.container_port
  protocol                 = "tcp"
  source_security_group_id = aws_security_group.alb_sg.id        # ALB SG
}

module "rds" {
  source                = "./modules/rds"
  db_name               = "shoppingcart"
  db_username           = "admin"
  db_password           = var.db_password
  subnet_ids            = module.network.private_subnet_ids
  vpc_id                = module.network.vpc_id
  ecs_security_group_id = module.network.security_group_id
}
module "dynamodb" {
  source = "./modules/dynamodb"
}