# Input variables. No secret has a value here — production secrets live in SSM
# Parameter Store under /waypoint/prod (created out of band; see README.md).

variable "aws_region" {
  description = "AWS region for every resource. us-east-1 keeps latency and cost predictable and matches the existing media bucket."
  type        = string
  default     = "us-east-1"
}

variable "project" {
  description = "Short project name used in resource names and tags."
  type        = string
  default     = "waypoint"
}

variable "environment" {
  description = "Deployment environment name, used in names/tags and the SSM path."
  type        = string
  default     = "prod"
}

variable "instance_type" {
  description = "EC2 instance type. t3.small (2 GB) comfortably runs the Go API and RabbitMQ together."
  type        = string
  default     = "t3.small"
}

variable "root_volume_size_gb" {
  description = "Size of the encrypted gp3 root volume in GiB."
  type        = number
  default     = 20
}

variable "ecr_repository_name" {
  description = "Name of the ECR repository that holds the API image."
  type        = string
  default     = "waypoint-api"
}

variable "s3_bucket_name" {
  description = "Existing private S3 bucket for media. Reused, never created or destroyed by this configuration."
  type        = string
  default     = "waypoint-project-media-985096928297-us-east-1-an"
}

variable "ssm_parameter_path" {
  description = "SSM Parameter Store path holding production configuration. The instance role may read only this path."
  type        = string
  default     = "/waypoint/prod"
}

variable "github_org" {
  description = "GitHub organisation/owner allowed to assume the deploy role through OIDC."
  type        = string
  default     = "Nihannn-18"
}

variable "github_repo" {
  description = "GitHub repository allowed to assume the deploy role through OIDC."
  type        = string
  default     = "Nexora_Waypoint"
}

variable "deploy_branches" {
  description = "Branches whose pushes may assume the deploy role."
  type        = list(string)
  default     = ["development", "main"]
}

variable "public_ingress_cidrs" {
  description = "CIDRs allowed to reach the API over HTTP/HTTPS. Caddy needs 80 (ACME + redirect) and 443 open to the world."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "compose_version" {
  description = "Pinned Docker Compose v2 plugin version installed on the instance."
  type        = string
  default     = "v2.32.4"
}

variable "tags" {
  description = "Additional tags merged onto every resource."
  type        = map(string)
  default     = {}
}
