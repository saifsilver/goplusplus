package main

func awsTerraformFiles() []scaffoldFile {
	return []scaffoldFile{
		{path: "versions.tf", content: awsVersions, mode: 0o644},
		{path: "variables.tf", content: awsVariables, mode: 0o644},
		{path: "main.tf", content: awsMain, mode: 0o644},
		{path: "networking.tf", content: awsNetworking, mode: 0o644},
		{path: "security.tf", content: awsSecurity, mode: 0o644},
		{path: "database.tf", content: awsDatabase, mode: 0o644},
		{path: "service.tf", content: awsService, mode: 0o644},
		{path: "outputs.tf", content: awsOutputs, mode: 0o644},
		{path: "terraform.tfvars.example", content: awsTFVars, mode: 0o644},
		{path: "backend.tf.example", content: awsBackend, mode: 0o644},
		{path: "deploy.sh", content: awsDeployScript, mode: 0o755},
		{path: ".gitignore", content: terraformGitignore, mode: 0o644},
		{path: "README.md", content: awsReadme, mode: 0o644},
	}
}

func standardHostingFiles() []scaffoldFile {
	return []scaffoldFile{
		{path: "compose.yaml", content: standardCompose, mode: 0o644},
		{path: "Caddyfile", content: standardCaddyfile, mode: 0o644},
		{path: ".env.example", content: standardEnv, mode: 0o644},
		{path: "secrets/.gitignore", content: standardSecretsGitignore, mode: 0o644},
		{path: "backups/.gitignore", content: standardBackupsGitignore, mode: 0o644},
		{path: "backup.sh", content: standardBackupScript, mode: 0o755},
		{path: "README.md", content: standardReadme, mode: 0o644},
	}
}

const awsVersions = `terraform {
  required_version = ">= 1.10.0, < 2.0.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}
`

const awsVariables = `variable "project_name" {
  description = "Short DNS-safe name used as the AWS resource prefix."
  type        = string
  default     = "{{APP_NAME}}"

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{2,23}$", var.project_name))
    error_message = "project_name must be 3-24 lowercase letters, numbers, or hyphens and start with a letter."
  }
}

variable "aws_region" {
  description = "AWS region for the workload."
  type        = string
  default     = "us-east-1"
}

variable "vpc_cidr" {
  description = "Private address range for the application VPC."
  type        = string
  default     = "10.40.0.0/16"
}

variable "certificate_arn" {
  description = "ACM certificate ARN in aws_region for the HTTPS listener."
  type        = string

  validation {
    condition     = can(regex("^arn:aws[a-z-]*:acm:", var.certificate_arn))
    error_message = "certificate_arn must be an ACM certificate ARN."
  }
}

variable "cors_allowed_origins" {
  description = "Exact browser origins allowed by CORS; wildcards are intentionally rejected by the app."
  type        = list(string)
  default     = []

  validation {
    condition     = !contains(var.cors_allowed_origins, "*")
    error_message = "cors_allowed_origins cannot contain a wildcard."
  }
}

variable "image_tag" {
  description = "Immutable ECR image tag to deploy, normally a Git commit SHA."
  type        = string
  default     = "bootstrap"

  validation {
    condition     = can(regex("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$", var.image_tag))
    error_message = "image_tag must be a valid container image tag."
  }
}

variable "desired_count" {
  type    = number
  default = 2
}

variable "minimum_count" {
  type    = number
  default = 2
}

variable "maximum_count" {
  type    = number
  default = 10
}

variable "task_cpu" {
  description = "Fargate task CPU units."
  type        = number
  default     = 512
}

variable "task_memory" {
  description = "Fargate task memory in MiB."
  type        = number
  default     = 1024
}

variable "database_name" {
  type    = string
  default = "app"

  validation {
    condition     = can(regex("^[A-Za-z][A-Za-z0-9_]*$", var.database_name))
    error_message = "database_name must be a valid PostgreSQL database name."
  }
}

variable "database_username" {
  type    = string
  default = "app_admin"

  validation {
    condition     = can(regex("^[A-Za-z][A-Za-z0-9_]*$", var.database_username))
    error_message = "database_username must be a valid PostgreSQL role name."
  }
}

variable "database_instance_class" {
  type    = string
  default = "db.t4g.small"
}

variable "database_multi_az" {
  description = "Keep enabled for production availability."
  type        = bool
  default     = true
}

variable "database_deletion_protection" {
  description = "Protect the database from accidental Terraform deletion."
  type        = bool
  default     = true
}

variable "tags" {
  type    = map(string)
  default = {}
}
`

const awsMain = `provider "aws" {
  region = var.aws_region

  default_tags {
    tags = merge({
      Application = var.project_name
      ManagedBy   = "terraform"
    }, var.tags)
  }
}

data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  name = var.project_name
  azs  = slice(data.aws_availability_zones.available.names, 0, 2)
}

check "availability_zones" {
  assert {
    condition     = length(data.aws_availability_zones.available.names) >= 2
    error_message = "The selected AWS region must expose at least two availability zones."
  }
}

check "scaling_bounds" {
  assert {
    condition     = var.minimum_count <= var.desired_count && var.desired_count <= var.maximum_count
    error_message = "minimum_count must be <= desired_count, which must be <= maximum_count."
  }
}
`

const awsNetworking = `resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = "${local.name}-vpc" }
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "${local.name}-igw" }
}

resource "aws_subnet" "public" {
  count = length(local.azs)

  vpc_id                  = aws_vpc.main.id
  availability_zone       = local.azs[count.index]
  cidr_block              = cidrsubnet(var.vpc_cidr, 8, count.index)
  map_public_ip_on_launch = false

  tags = { Name = "${local.name}-public-${local.azs[count.index]}" }
}

resource "aws_subnet" "private" {
  count = length(local.azs)

  vpc_id            = aws_vpc.main.id
  availability_zone = local.azs[count.index]
  cidr_block        = cidrsubnet(var.vpc_cidr, 8, count.index + 10)

  tags = { Name = "${local.name}-private-${local.azs[count.index]}" }
}

resource "aws_eip" "nat" {
  count  = length(local.azs)
  domain = "vpc"

  depends_on = [aws_internet_gateway.main]
  tags       = { Name = "${local.name}-nat-${local.azs[count.index]}" }
}

resource "aws_nat_gateway" "main" {
  count = length(local.azs)

  allocation_id = aws_eip.nat[count.index].id
  subnet_id     = aws_subnet.public[count.index].id

  depends_on = [aws_internet_gateway.main]
  tags       = { Name = "${local.name}-nat-${local.azs[count.index]}" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }

  tags = { Name = "${local.name}-public" }
}

resource "aws_route_table_association" "public" {
  count = length(local.azs)

  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table" "private" {
  count = length(local.azs)

  vpc_id = aws_vpc.main.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.main[count.index].id
  }

  tags = { Name = "${local.name}-private-${local.azs[count.index]}" }
}

resource "aws_route_table_association" "private" {
  count = length(local.azs)

  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private[count.index].id
}
`

const awsSecurity = `resource "aws_security_group" "load_balancer" {
  name        = "${local.name}-alb"
  description = "Public HTTPS ingress"
  vpc_id      = aws_vpc.main.id

  ingress {
    description = "HTTP redirect"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description = "HTTPS"
    from_port   = 443
    to_port     = 443
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

resource "aws_security_group" "service" {
  name        = "${local.name}-service"
  description = "Application tasks accept traffic only from the ALB"
  vpc_id      = aws_vpc.main.id

  ingress {
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.load_balancer.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "database" {
  name        = "${local.name}-database"
  description = "PostgreSQL accepts traffic only from application tasks"
  vpc_id      = aws_vpc.main.id

  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.service.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
`

const awsDatabase = `resource "aws_db_subnet_group" "main" {
  name       = "${local.name}-database"
  subnet_ids = aws_subnet.private[*].id
}

resource "aws_db_instance" "main" {
  identifier = "${local.name}-postgres"

  engine         = "postgres"
  instance_class = var.database_instance_class
  db_name        = var.database_name
  username       = var.database_username

  manage_master_user_password = true
  storage_encrypted            = true
  storage_type                 = "gp3"
  allocated_storage            = 20
  max_allocated_storage        = 200

  multi_az               = var.database_multi_az
  publicly_accessible    = false
  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.database.id]

  backup_retention_period   = 14
  auto_minor_version_upgrade = true
  copy_tags_to_snapshot      = true
  deletion_protection        = var.database_deletion_protection
  skip_final_snapshot        = false
  final_snapshot_identifier  = "${local.name}-final"
}
`

const awsService = `resource "aws_ecr_repository" "app" {
  name                 = local.name
  image_tag_mutability = "IMMUTABLE"
  force_delete         = false

  encryption_configuration {
    encryption_type = "AES256"
  }

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_lifecycle_policy" "app" {
  repository = aws_ecr_repository.app.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Retain the newest 30 images"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 30
      }
      action = { type = "expire" }
    }]
  })
}

resource "aws_cloudwatch_log_group" "app" {
  name              = "/ecs/${local.name}"
  retention_in_days = 30
}

data "aws_iam_policy_document" "ecs_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "${local.name}-ecs-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

data "aws_iam_policy_document" "database_secret" {
  statement {
    actions   = ["secretsmanager:GetSecretValue"]
    resources = [aws_db_instance.main.master_user_secret[0].secret_arn]
  }
}

resource "aws_iam_role_policy" "database_secret" {
  name   = "database-secret"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.database_secret.json
}

resource "aws_iam_role" "task" {
  name               = "${local.name}-ecs-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json
}

resource "aws_ecs_cluster" "app" {
  name = local.name

  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}

resource "aws_lb" "app" {
  name               = local.name
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.load_balancer.id]
  subnets            = aws_subnet.public[*].id
  drop_invalid_header_fields = true
}

resource "aws_lb_target_group" "app" {
  name        = local.name
  port        = 8080
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = aws_vpc.main.id

  deregistration_delay = 30

  health_check {
    path                = "/health/ready"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    timeout             = 5
    interval            = 15
    matcher             = "200"
  }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.app.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.app.arn
  port              = 443
  protocol          = "HTTPS"
  certificate_arn   = var.certificate_arn
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }
}

resource "aws_ecs_task_definition" "app" {
  family                   = local.name
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.task_cpu
  memory                   = var.task_memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  container_definitions = jsonencode([{
    name      = local.name
    image     = "${aws_ecr_repository.app.repository_url}:${var.image_tag}"
    essential = true
    portMappings = [{
      containerPort = 8080
      hostPort      = 8080
      protocol      = "tcp"
    }]
    environment = [
      { name = "APP_ENV", value = "production" },
      { name = "HTTP_ADDRESS", value = ":8080" },
      { name = "DB_HOST", value = aws_db_instance.main.address },
      { name = "DB_PORT", value = tostring(aws_db_instance.main.port) },
      { name = "DB_NAME", value = var.database_name },
      { name = "DB_USER", value = var.database_username },
      { name = "DB_SSLMODE", value = "require" },
      { name = "CORS_ALLOWED_ORIGINS", value = join(",", var.cors_allowed_origins) }
    ]
    secrets = [{
      name      = "DB_PASSWORD"
      valueFrom = "${aws_db_instance.main.master_user_secret[0].secret_arn}:password::"
    }]
    healthCheck = {
      command     = ["CMD-SHELL", "wget -qO- http://127.0.0.1:8080/health/live || exit 1"]
      interval    = 30
      timeout     = 5
      retries     = 3
      startPeriod = 20
    }
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.app.name
        awslogs-region        = var.aws_region
        awslogs-stream-prefix = "app"
      }
    }
  }])
}

resource "aws_ecs_service" "app" {
  name            = local.name
  cluster         = aws_ecs_cluster.app.id
  task_definition = aws_ecs_task_definition.app.arn
  desired_count   = var.desired_count

  capacity_provider_strategy {
    capacity_provider = "FARGATE"
    weight            = 1
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  deployment_maximum_percent         = 200
  deployment_minimum_healthy_percent = 100
  health_check_grace_period_seconds  = 60
  enable_execute_command             = false

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.service.id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.app.arn
    container_name   = local.name
    container_port   = 8080
  }

  depends_on = [aws_lb_listener.https]

  lifecycle {
    ignore_changes = [desired_count]
  }
}

resource "aws_appautoscaling_target" "app" {
  min_capacity       = var.minimum_count
  max_capacity       = var.maximum_count
  resource_id        = "service/${aws_ecs_cluster.app.name}/${aws_ecs_service.app.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}

resource "aws_appautoscaling_policy" "cpu" {
  name               = "${local.name}-cpu"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.app.resource_id
  scalable_dimension = aws_appautoscaling_target.app.scalable_dimension
  service_namespace  = aws_appautoscaling_target.app.service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value       = 60
    scale_in_cooldown  = 120
    scale_out_cooldown = 30
  }
}

resource "aws_appautoscaling_policy" "memory" {
  name               = "${local.name}-memory"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.app.resource_id
  scalable_dimension = aws_appautoscaling_target.app.scalable_dimension
  service_namespace  = aws_appautoscaling_target.app.service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageMemoryUtilization"
    }
    target_value       = 70
    scale_in_cooldown  = 120
    scale_out_cooldown = 30
  }
}
`

const awsOutputs = `output "load_balancer_url" {
  value       = "https://${aws_lb.app.dns_name}"
  description = "Point your application DNS record at this load balancer."
}

output "ecr_repository_url" {
  value = aws_ecr_repository.app.repository_url
}

output "database_secret_arn" {
  value       = aws_db_instance.main.master_user_secret[0].secret_arn
  description = "AWS-managed database credentials; the secret value is not stored in Terraform configuration."
}
`

const awsTFVars = `project_name      = "{{APP_NAME}}"
aws_region       = "us-east-1"
certificate_arn  = "arn:aws:acm:us-east-1:123456789012:certificate/replace-me"
cors_allowed_origins = ["https://app.example.com"]

tags = {
  Environment = "production"
  Owner       = "platform"
}
`

const awsBackend = `terraform {
  backend "s3" {
    bucket       = "replace-with-versioned-state-bucket"
    key          = "{{APP_NAME}}/production/terraform.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}
`

const awsDeployScript = `#!/bin/sh
set -eu

if [ ! -f terraform.tfvars ]; then
  echo "terraform.tfvars is required; copy terraform.tfvars.example and configure it" >&2
  exit 1
fi

image_tag="${IMAGE_TAG:-$(git rev-parse --short=12 HEAD)}"
aws_region="${AWS_REGION:-us-east-1}"

if [ ! -f backend.tf ]; then
  echo "warning: backend.tf is absent; local Terraform state is unsuitable for team production use" >&2
fi

terraform init

if ! terraform state show aws_ecr_repository.app >/dev/null 2>&1; then
  terraform apply -target=aws_ecr_repository.app -target=aws_ecr_lifecycle_policy.app
fi

repository="$(terraform output -raw ecr_repository_url)"
registry="${repository%%/*}"
aws ecr get-login-password --region "$aws_region" | docker login --username AWS --password-stdin "$registry"
docker build --pull -t "$repository:$image_tag" ../../..
docker push "$repository:$image_tag"

terraform apply -var="image_tag=$image_tag"
`

const terraformGitignore = `.terraform/
*.tfstate
*.tfstate.*
*.tfplan
terraform.tfvars
crash.log
`

const awsReadme = `# AWS scalable hosting

This Terraform stack runs {{APP_NAME}} on private ECS Fargate tasks behind an HTTPS Application Load Balancer. It creates two-AZ networking, one NAT gateway per AZ, an encrypted Multi-AZ PostgreSQL RDS instance, an immutable ECR repository, CloudWatch logs, deployment rollback, and CPU/memory autoscaling.

## Prerequisites

- Terraform 1.10+
- AWS CLI authenticated through a role or standard credentials chain
- Docker
- An ACM certificate in the selected region

Never put AWS keys or database passwords in Terraform variables. RDS manages its master password in Secrets Manager and the ECS execution role can read only that secret.

## Deploy

` + "```bash" + `
cd deploy/terraform/aws
cp terraform.tfvars.example terraform.tfvars
# Edit certificate_arn, origins, sizing, and tags.
./deploy.sh
` + "```" + `

On the first run, the script bootstraps only ECR, pushes an immutable image, and then applies the complete stack. Later runs leave the existing ECS service live while the new image is built and pushed.

For team use, copy ` + "`backend.tf.example`" + ` to ` + "`backend.tf`" + ` and configure a versioned S3 bucket. The generated backend uses native S3 state locking. Protect state access with least-privilege IAM.

## Operations

- Point DNS to the ` + "`load_balancer_url`" + ` output.
- Keep ` + "`database_deletion_protection = true`" + ` in production.
- Review NAT gateway, Multi-AZ RDS, log ingestion, and Fargate costs before applying.
- Use a new image tag for every release. Reusing tags is blocked by ECR immutability.
- Add application-specific IAM permissions to the task role, never the execution role.
`

const standardCompose = `services:
  proxy:
    image: caddy:2.10-alpine
    restart: unless-stopped
    environment:
      DOMAIN: ${DOMAIN:?set DOMAIN in .env}
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      app:
        condition: service_healthy
    networks: [edge]

  app:
    build:
      context: ../..
      dockerfile: Dockerfile
    restart: unless-stopped
    environment:
      APP_ENV: production
      HTTP_ADDRESS: :8080
      DB_HOST: database
      DB_PORT: "5432"
      DB_NAME: ${POSTGRES_DB:-app}
      DB_USER: ${POSTGRES_USER:-app_admin}
      DB_PASSWORD_FILE: /run/secrets/postgres_password
      DB_SSLMODE: disable
      CORS_ALLOWED_ORIGINS: ${CORS_ALLOWED_ORIGINS:?set CORS_ALLOWED_ORIGINS in .env}
    secrets: [postgres_password]
    depends_on:
      database:
        condition: service_healthy
        restart: true
    expose: ["8080"]
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:8080/health/ready || exit 1"]
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 20s
    read_only: true
    tmpfs: [/tmp]
    cap_drop: [ALL]
    security_opt: [no-new-privileges:true]
    networks: [edge, private]

  database:
    image: postgres:18-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: ${POSTGRES_DB:-app}
      POSTGRES_USER: ${POSTGRES_USER:-app_admin}
      POSTGRES_PASSWORD_FILE: /run/secrets/postgres_password
    secrets: [postgres_password]
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $${POSTGRES_USER} -d $${POSTGRES_DB}"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 20s
    networks: [private]

secrets:
  postgres_password:
    file: ./secrets/postgres_password

volumes:
  postgres_data:
  caddy_data:
  caddy_config:

networks:
  edge:
  private:
    internal: true
`

const standardCaddyfile = `{$DOMAIN} {
	encode zstd gzip

	reverse_proxy app:8080 {
		health_uri /health/ready
		health_interval 10s
		health_timeout 3s
		lb_try_duration 5s
	}

	log {
		output stdout
		format json
	}
}
`

const standardEnv = `DOMAIN=app.example.com
POSTGRES_DB=app
POSTGRES_USER=app_admin
CORS_ALLOWED_ORIGINS=https://app.example.com
`

const standardSecretsGitignore = `*
!.gitignore
`

const standardBackupsGitignore = `*
!.gitignore
`

const standardBackupScript = `#!/bin/sh
set -eu

script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
cd "$script_directory"

if [ ! -f .env ]; then
  echo ".env is required" >&2
  exit 1
fi

set -a
. ./.env
set +a

mkdir -p backups
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
docker compose exec -T database pg_dump \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" \
  --format=custom > "backups/${POSTGRES_DB}-${timestamp}.dump"
`

const standardReadme = `# Standard VPS hosting

This target runs {{APP_NAME}}, PostgreSQL, and Caddy on one Linux host. Caddy obtains and renews HTTPS certificates automatically. PostgreSQL is reachable only on the private Compose network, and the application receives its database password through a mounted secret file.

## Requirements

- A Linux VPS with Docker Engine and Docker Compose
- DNS for your domain pointed at the VPS
- Inbound TCP 80/443 and UDP 443 allowed by the host firewall

## Deploy

` + "```bash" + `
cd deploy/standard
cp .env.example .env
# Edit DOMAIN and CORS_ALLOWED_ORIGINS.
umask 077
openssl rand -base64 36 > secrets/postgres_password
docker compose config
docker compose up -d --build
docker compose ps
` + "```" + `

Do not commit ` + "`.env`" + `, secret files, database data, or backups. Back up both the database and Caddy volumes through your host or infrastructure provider.

## Database backup

` + "```bash" + `
./backup.sh
` + "```" + `

Copy backups off the VPS and periodically test restoration. A single VPS is operationally simple but is not highly available; use the AWS Terraform target when you need multi-AZ database availability and horizontal application scaling.
`
