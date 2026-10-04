locals {
  name_prefix = "${var.project}-${var.environment}"

  tags = merge(
    {
      Project     = var.project
      Environment = var.environment
      ManagedBy   = "terraform"
    },
    var.tags,
  )

  # The API is reached over TLS on a free sslip.io hostname derived from the
  # Elastic IP — no domain purchase, a real Let's Encrypt certificate issued by
  # Caddy on the instance.
  api_hostname   = "${aws_eip.waypoint.public_ip}.sslip.io"
  api_base_url   = "https://${local.api_hostname}/api/v1"
  api_health_url = "https://${local.api_hostname}/readyz"

  deploy_dir = "${path.module}/../deploy"

  # The instance's bootstrap artifacts come from the same files the deploy
  # workflow uses, so there is a single source of truth and no drift.
  compose_file      = file("${local.deploy_dir}/docker-compose.prod.yml")
  caddy_file        = file("${local.deploy_dir}/Caddyfile")
  render_env_script = file("${local.deploy_dir}/render-env.sh")
  deploy_script     = file("${local.deploy_dir}/waypoint-deploy.sh")
  service_file      = file("${local.deploy_dir}/waypoint.service")

  # Non-secret interpolation values for Docker Compose. Secrets never appear
  # here; they are rendered from SSM Parameter Store at deploy time.
  config_env = <<-EOT
AWS_REGION=${var.aws_region}
SSM_PARAMETER_PATH=${var.ssm_parameter_path}
ECR_REPO=${aws_ecr_repository.api.repository_url}
WAYPOINT_API_HOST=${local.api_hostname}
EOT
}
