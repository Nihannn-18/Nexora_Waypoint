output "aws_region" {
  description = "Region of the deployment."
  value       = var.aws_region
}

output "ecr_repository_url" {
  description = "ECR repository the deploy workflow pushes the API image to. Set this as the GitHub variable ECR_REPOSITORY."
  value       = aws_ecr_repository.api.repository_url
}

output "ec2_instance_id" {
  description = "Instance id. Set this as the GitHub variable EC2_INSTANCE_ID."
  value       = aws_instance.api.id
}

output "elastic_ip" {
  description = "Static public IPv4 address attached to the API instance."
  value       = aws_eip.waypoint.public_ip
}

output "api_hostname" {
  description = "Free sslip.io hostname Caddy obtains a Let's Encrypt certificate for."
  value       = local.api_hostname
}

output "api_base_url" {
  description = "HTTPS API base URL used by the Vercel frontend (NEXT_PUBLIC_API_BASE_URL)."
  value       = local.api_base_url
}

output "api_health_url" {
  description = "Readiness URL for the post-deploy health check. Set this as the GitHub variable API_HEALTH_URL."
  value       = local.api_health_url
}

output "github_deploy_role_arn" {
  description = "OIDC role GitHub Actions assumes. Set this as the GitHub variable AWS_DEPLOY_ROLE_ARN."
  value       = aws_iam_role.github_deploy.arn
}

output "instance_role_arn" {
  description = "Least-privilege EC2 instance role."
  value       = aws_iam_role.instance.arn
}

output "security_group_id" {
  description = "Edge security group (80/443 only)."
  value       = aws_security_group.api.id
}

output "media_bucket" {
  description = "Existing private S3 bucket reused for media."
  value       = var.s3_bucket_name
}
