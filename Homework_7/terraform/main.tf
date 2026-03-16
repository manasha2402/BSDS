# Wire together four focused modules: network, ecr, logging, ecs.

module "network" {
  source         = "./modules/network"
  service_name   = var.service_name
  container_port = var.container_port
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

# Order Receiver — handles /sync and /async, needs ALB for incoming HTTP traffic
module "ecs_receiver" {
  source                = "./modules/ecs"
  service_name          = "order-receiver"
  image                 = "${module.ecr.repository_url}:order-receiver"
  container_port        = var.container_port
  subnet_ids            = module.network.subnet_ids
  security_group_ids    = [module.network.security_group_id]
  execution_role_arn    = data.aws_iam_role.lab_role.arn
  task_role_arn         = data.aws_iam_role.lab_role.arn
  log_group_name        = module.logging.log_group_name
  ecs_count             = 1
  region                = var.aws_region
  vpc_id                = module.network.vpc_id
  alb_security_group_id = module.network.alb_security_group_id
  min_instances         = 1
  max_instances         = 4
  cpu_target            = var.cpu_target
  scale_cooldown        = var.scale_cooldown
  enable_alb            = true
  sns_topic_arn         = module.sns.topic_arn
  sqs_queue_url         = module.sqs.queue_url
}

# Order Processor — polls SQS, no ALB needed
module "ecs_processor" {
  source             = "./modules/ecs"
  service_name       = "order-processor"
  image              = "${module.ecr.repository_url}:order-processor"
  container_port     = var.container_port
  subnet_ids         = module.network.subnet_ids
  security_group_ids = [module.network.security_group_id]
  execution_role_arn = data.aws_iam_role.lab_role.arn
  task_role_arn      = data.aws_iam_role.lab_role.arn
  log_group_name     = module.logging.log_group_name
  ecs_count          = 1
  region             = var.aws_region
  vpc_id             = module.network.vpc_id
  min_instances      = 1
  max_instances      = 4
  cpu_target         = var.cpu_target
  scale_cooldown     = var.scale_cooldown
  enable_alb         = false
  sns_topic_arn      = module.sns.topic_arn
  sqs_queue_url      = module.sqs.queue_url
}

# SQS must come first — SNS needs the queue ARN to create the subscription
module "sqs" {
  source        = "./modules/sqs"
  queue_name    = "order-processing-queue"
  sns_topic_arn = module.sns.topic_arn
}

module "sns" {
  source        = "./modules/sns"
  topic_name    = "order-processing-events"
  sqs_queue_arn = module.sqs.queue_arn
}

# Build & push order-receiver image
resource "docker_image" "receiver" {
  name = "${module.ecr.repository_url}:order-receiver"
  build {
    context    = "../src"
    dockerfile = "order-receiver/Dockerfile"
  }
}

resource "docker_registry_image" "receiver" {
  name = docker_image.receiver.name
}

# Build & push order-processor image
resource "docker_image" "processor" {
  name = "${module.ecr.repository_url}:order-processor"
  build {
    context    = "../src"
    dockerfile = "order-processor/Dockerfile"
  }
}

resource "docker_registry_image" "processor" {
  name = docker_image.processor.name
}