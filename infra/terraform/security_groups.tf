# Public surface is deliberately tiny: 80/443 for Caddy, nothing else. There is
# no SSH rule (use SSM Session Manager); the Go API, RabbitMQ and their ports are
# reachable only inside the instance's Docker network.

resource "aws_security_group" "api" {
  name        = "${local.name_prefix}-edge"
  description = "Waypoint API edge: HTTP/HTTPS from the internet, no SSH."
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "HTTP for ACME challenge and HTTPS redirect"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = var.public_ingress_cidrs
  }

  ingress {
    description = "HTTPS"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = var.public_ingress_cidrs
  }

  egress {
    description = "Outbound for ECR, SSM, Neon, S3 and ACME"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.tags, { Name = "${local.name_prefix}-edge" })
}
