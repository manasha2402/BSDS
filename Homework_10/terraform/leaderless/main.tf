# ---------------------------------------------------------------------------
# HW10 — Leaderless KV Store on AWS
# 5 identical Fargate tasks, any node accepts reads or writes
# Cloud Map for inter-node DNS, one ALB per node for external access
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
  name        = "${var.service_name}-ll-nodes-sg"
  description = "Allow inbound on 8080 for leaderless KV nodes"
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
  name        = "${var.service_name}-ll-alb-sg"
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
  name         = "${var.service_name}-ll-node"
  force_delete = true
}

resource "docker_image" "kv" {
  name = "${aws_ecr_repository.kv.repository_url}:latest"
  build {
    context = "../../leaderless"
  }
}

resource "docker_registry_image" "kv" {
  name = docker_image.kv.name
}

# ── CloudWatch Logs ────────────────────────────────────────────────────────

resource "aws_cloudwatch_log_group" "kv" {
  name              = "/ecs/${var.service_name}-leaderless"
  retention_in_days = var.log_retention_days
}

# ── ECS Cluster ────────────────────────────────────────────────────────────

resource "aws_ecs_cluster" "kv" {
  name = "${var.service_name}-ll-cluster"
}

# ── Cloud Map ──────────────────────────────────────────────────────────────

resource "aws_service_discovery_private_dns_namespace" "kv" {
  name        = "${var.service_name}-ll.local"
  description = "Private DNS for leaderless KV nodes"
  vpc         = data.aws_vpc.default.id
}

resource "aws_service_discovery_service" "node" {
  count = 5
  name  = "node${count.index + 1}"

  dns_config {
    namespace_id   = aws_service_discovery_private_dns_namespace.kv.id
    routing_policy = "MULTIVALUE"
    dns_records {
      ttl  = 10
      type = "A"
    }
  }

  health_check_custom_config {
    failure_threshold = 1
  }
}

# ── Task Definitions ───────────────────────────────────────────────────────

locals {
  # Each node's PEERS = all other nodes via Cloud Map DNS
  node_peers = [
    for i in range(5) : join(",", [
      for j in range(5) : "http://node${j + 1}.${var.service_name}-ll.local:${var.container_port}"
      if j != i  # exclude self
    ])
  ]
}

resource "aws_ecs_task_definition" "node" {
  count                    = 5
  family                   = "${var.service_name}-ll-node${count.index + 1}"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = data.aws_iam_role.lab_role.arn
  task_role_arn            = data.aws_iam_role.lab_role.arn

  container_definitions = jsonencode([{
    name      = "node${count.index + 1}"
    image     = "${aws_ecr_repository.kv.repository_url}:latest"
    essential = true
    portMappings = [{
      containerPort = var.container_port
    }]
    environment = [
      { name = "NODE_ID", value = "node${count.index + 1}" },
      { name = "PORT",    value = tostring(var.container_port) },
      { name = "PEERS",   value = local.node_peers[count.index] }
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.kv.name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "node${count.index + 1}"
      }
    }
  }])

  depends_on = [docker_registry_image.kv]
}

# ── ALBs (one per node) ────────────────────────────────────────────────────

resource "aws_lb" "node" {
  count              = 5
  name               = "${var.service_name}-ll-node${count.index + 1}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = data.aws_subnets.default.ids
}

resource "aws_lb_target_group" "node" {
  count        = 5
  name         = "${var.service_name}-ll-n${count.index + 1}-tg"
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

resource "aws_lb_listener" "node" {
  count             = 5
  load_balancer_arn = aws_lb.node[count.index].arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.node[count.index].arn
  }
}

# ── ECS Services ───────────────────────────────────────────────────────────

resource "aws_ecs_service" "node" {
  count           = 5
  name            = "${var.service_name}-ll-node${count.index + 1}"
  cluster         = aws_ecs_cluster.kv.id
  task_definition = aws_ecs_task_definition.node[count.index].arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = data.aws_subnets.default.ids
    security_groups  = [aws_security_group.nodes.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.node[count.index].arn
    container_name   = "node${count.index + 1}"
    container_port   = var.container_port
  }

  service_registries {
    registry_arn = aws_service_discovery_service.node[count.index].arn
  }

  depends_on = [aws_lb_listener.node]
}