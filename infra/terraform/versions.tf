# Terraform and provider versions for the Waypoint production infrastructure.
#
# State is local by design: this is a single, low-traffic hackathon deployment
# and adding an S3/DynamoDB backend would create billable resources that the
# stack does not otherwise need. `*.tfstate` is gitignored. See README.md.

terraform {
  required_version = ">= 1.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = local.tags
  }
}
