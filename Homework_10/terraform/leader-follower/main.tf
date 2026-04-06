# ---------------------------------------------------------------------------
# HW10 — Distributed KV Store on AWS
# One ECS cluster, 5 Fargate tasks (leader + 4 followers)
# No Cloud Map — PEERS are set manually after deploy
# ---------------------------------------------------------------------------

# ── Networking ─────────────────────────────────────────────────────────────

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

resource "aws_security_group" "nodes" {
  name        = "${var.service_name}-nodes-sg"
  description = "Allow inbound on 8080 for KV nodes"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    from_port   = var.container_port
    to_port     = var.container_port
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

resource "aws_security_group" "alb" {
  name        = "${var.service_name}-alb-sg"
  description = "Allow inbound HTTP on port 80"
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

# ── IAM ────────────────────────────────────────────────────────────────────

data "aws_iam_role" "lab_role" {
  name = "LabRole"
}

# ── ECR ────────────────────────────────────────────────────────────────────

resource "aws_ecr_repository" "kv" {
  name         = "${var.service_name}-node"
  force_delete = true
}

resource "docker_image" "kv" {
  name = "${aws_ecr_repository.kv.repository_url}:latest"
  build {
    context = "../../leader-follower"
  }
}

resource "docker_registry_image" "kv" {
  name = docker_image.kv.name
}

# ── CloudWatch Logs ────────────────────────────────────────────────────────

resource "aws_cloudwatch_log_group" "kv" {
  name              = "/ecs/${var.service_name}"
  retention_in_days = var.log_retention_days
}

# ── ECS Cluster ────────────────────────────────────────────────────────────

resource "aws_ecs_cluster" "kv" {
  name = "${var.service_name}-cluster"
}

# ── Task Definitions ───────────────────────────────────────────────────────

# Leader task — PEERS is empty for now, updated manually after deploy
resource "aws_ecs_task_definition" "leader" {
  family                   = "${var.service_name}-leader"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = data.aws_iam_role.lab_role.arn
  task_role_arn            = data.aws_iam_role.lab_role.arn

  container_definitions = jsonencode([{
    name      = "leader"
    image     = "${aws_ecr_repository.kv.repository_url}:latest"
    essential = true
    portMappings = [{
      containerPort = var.container_port
    }]
    environment = [
      { name = "ROLE",  value = "leader" },
      { name = "PORT",  value = tostring(var.container_port) },
      { name = "PEERS", value = var.peers },
      { name = "W",     value = tostring(var.W) },
      { name = "R",     value = tostring(var.R) }
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.kv.name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "leader"
      }
    }
  }])

  depends_on = [docker_registry_image.kv]
}

# Follower task definitions
resource "aws_ecs_task_definition" "follower" {
  count                    = 4
  family                   = "${var.service_name}-follower${count.index + 1}"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = data.aws_iam_role.lab_role.arn
  task_role_arn            = data.aws_iam_role.lab_role.arn

  container_definitions = jsonencode([{
    name      = "follower${count.index + 1}"
    image     = "${aws_ecr_repository.kv.repository_url}:latest"
    essential = true
    portMappings = [{
      containerPort = var.container_port
    }]
    environment = [
      { name = "ROLE", value = "follower" },
      { name = "PORT", value = tostring(var.container_port) },
      { name = "W",    value = tostring(var.W) },
      { name = "R",    value = tostring(var.R) },
      { name = "PEERS", value = var.follower_peers }
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.kv.name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "follower${count.index + 1}"
      }
    }
  }])

  depends_on = [docker_registry_image.kv]
}

# ── ALBs ───────────────────────────────────────────────────────────────────

resource "aws_lb" "leader" {
  name               = "${var.service_name}-leader-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = data.aws_subnets.default.ids
}

resource "aws_lb_target_group" "leader" {
  name        = "${var.service_name}-leader-tg"
  port        = var.container_port
  protocol    = "HTTP"
  vpc_id      = data.aws_vpc.default.id
  target_type = "ip"
  health_check {
    path                = "/health"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
    matcher             = "200"
  }
}

resource "aws_lb_listener" "leader" {
  load_balancer_arn = aws_lb.leader.arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.leader.arn
  }
}

resource "aws_lb" "follower" {
  count              = 4
  name               = "${var.service_name}-f${count.index + 1}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = data.aws_subnets.default.ids
}

resource "aws_lb_target_group" "follower" {
  count        = 4
  name         = "${var.service_name}-f${count.index + 1}-tg"
  port         = var.container_port
  protocol     = "HTTP"
  vpc_id       = data.aws_vpc.default.id
  target_type  = "ip"
  health_check {
    path                = "/health"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
    matcher             = "200"
  }
}

resource "aws_lb_listener" "follower" {
  count             = 4
  load_balancer_arn = aws_lb.follower[count.index].arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.follower[count.index].arn
  }
}

# ── ECS Services ───────────────────────────────────────────────────────────

resource "aws_ecs_service" "leader" {
  name            = "${var.service_name}-leader"
  cluster         = aws_ecs_cluster.kv.id
  task_definition = aws_ecs_task_definition.leader.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = data.aws_subnets.default.ids
    security_groups  = [aws_security_group.nodes.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.leader.arn
    container_name   = "leader"
    container_port   = var.container_port
  }

  depends_on = [aws_lb_listener.leader]
}

resource "aws_ecs_service" "follower" {
  count           = 4
  name            = "${var.service_name}-follower${count.index + 1}"
  cluster         = aws_ecs_cluster.kv.id
  task_definition = aws_ecs_task_definition.follower[count.index].arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = data.aws_subnets.default.ids
    security_groups  = [aws_security_group.nodes.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.follower[count.index].arn
    container_name   = "follower${count.index + 1}"
    container_port   = var.container_port
  }

  depends_on = [aws_lb_listener.follower]
}