# EC2 instance role: least privilege. It reads only /waypoint/prod parameters,
# pulls only the API image from ECR, and touches only the one media bucket.
# No administrator access, no long-lived keys.

data "aws_iam_policy_document" "ec2_assume" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "instance" {
  name               = "${local.name_prefix}-ec2"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume.json
}

# Session Manager access without opening SSH.
resource "aws_iam_role_policy_attachment" "ssm_core" {
  role       = aws_iam_role.instance.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

data "aws_iam_policy_document" "instance" {
  statement {
    sid = "ReadProductionParameters"

    actions = [
      "ssm:GetParameter",
      "ssm:GetParameters",
      "ssm:GetParametersByPath",
    ]

    resources = [
      "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter${var.ssm_parameter_path}",
      "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter${var.ssm_parameter_path}/*",
    ]
  }

  # SecureString parameters use the AWS-managed ssm key. Decryption is allowed
  # only when the request comes through SSM.
  statement {
    sid       = "DecryptSecureParameters"
    actions   = ["kms:Decrypt"]
    resources = ["*"]

    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values   = ["ssm.${var.aws_region}.amazonaws.com"]
    }
  }

  statement {
    sid       = "EcrAuth"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }

  statement {
    sid = "EcrPullApiImage"

    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
    ]

    resources = [aws_ecr_repository.api.arn]
  }

  statement {
    sid = "MediaBucketObjects"

    actions = [
      "s3:GetObject",
      "s3:PutObject",
      "s3:DeleteObject",
    ]

    resources = ["arn:aws:s3:::${var.s3_bucket_name}/*"]
  }
}

resource "aws_iam_role_policy" "instance" {
  name   = "${local.name_prefix}-instance"
  role   = aws_iam_role.instance.id
  policy = data.aws_iam_policy_document.instance.json
}

resource "aws_iam_instance_profile" "instance" {
  name = "${local.name_prefix}-ec2"
  role = aws_iam_role.instance.name
}
