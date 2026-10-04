# Look-ups against the existing account. Nothing here creates or changes a
# resource; the default VPC is reused deliberately to avoid an unnecessary
# NAT gateway and the extra cost of a bespoke network.

data "aws_caller_identity" "current" {}

data "aws_region" "current" {}

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

data "aws_subnet" "selected" {
  id = tolist(data.aws_subnets.default.ids)[0]
}

# Latest Amazon Linux 2023 x86_64 (matches the amd64 image built in CI and the
# t3.small instance). AL2023 ships AWS CLI v2.
data "aws_ami" "al2023" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-2023.*-x86_64"]
  }

  filter {
    name   = "architecture"
    values = ["x86_64"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}
