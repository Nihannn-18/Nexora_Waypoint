# Waypoint production infrastructure (Terraform)

Minimal AWS footprint for the hackathon deployment:

- **ECR** repository for the Go API image (SHA-tagged).
- **EC2** `t3.small` (amd64) in the **default VPC**, with an **Elastic IP**.
- **Security group**: 80/443 only. No SSH — use SSM Session Manager.
- **IAM** EC2 instance role (least privilege) and a **GitHub OIDC** deploy role.
- A free **sslip.io** hostname + Caddy-issued Let's Encrypt certificate for HTTPS.

Not created here (and deliberately so): VPC/NAT/ALB, RDS, ECS/EKS/Fargate, a new
S3 bucket, or a custom domain. PostgreSQL stays on **Neon**; media stays in the
existing private **S3** bucket; the web app runs on **Vercel**.

## State

Terraform state is **local** (`terraform.tfstate`, gitignored). There is no
remote backend so the stack creates no billable S3/DynamoDB state resources. Run
this from one machine; coordinate before applying from elsewhere.

## Secrets

No secret is defined in this configuration. Production secrets live in SSM
Parameter Store under **`/waypoint/prod`**, created out of band:

```bash
# Example shape only — run from a secure shell; the values are never echoed.
aws ssm put-parameter --region us-east-1 --type SecureString \
  --name /waypoint/prod/DATABASE_URL --value "$DATABASE_URL"
aws ssm put-parameter --region us-east-1 --type SecureString \
  --name /waypoint/prod/RABBITMQ_URL --value "$RABBITMQ_URL"
aws ssm put-parameter --region us-east-1 --type SecureString \
  --name /waypoint/prod/RABBITMQ_PASSWORD --value "$RABBITMQ_PASSWORD"
aws ssm put-parameter --region us-east-1 --type String \
  --name /waypoint/prod/RABBITMQ_USER --value "waypoint"
aws ssm put-parameter --region us-east-1 --type SecureString \
  --name /waypoint/prod/DEMO_SEED_PASSWORD --value "<demo password>"
aws ssm put-parameter --region us-east-1 --type String \
  --name /waypoint/prod/APP_ENV --value "production"
aws ssm put-parameter --region us-east-1 --type String \
  --name /waypoint/prod/CORS_ORIGIN --value "https://<vercel-origin>"
aws ssm put-parameter --region us-east-1 --type String \
  --name /waypoint/prod/MEDIA_STORAGE --value "s3"
aws ssm put-parameter --region us-east-1 --type String \
  --name /waypoint/prod/S3_BUCKET --value "waypoint-project-media-985096928297-us-east-1-an"
aws ssm put-parameter --region us-east-1 --type String \
  --name /waypoint/prod/AWS_REGION --value "us-east-1"
aws ssm put-parameter --region us-east-1 --type String \
  --name /waypoint/prod/SESSION_TTL --value "12h"
aws ssm put-parameter --region us-east-1 --type String \
  --name /waypoint/prod/DEMO_MODE --value "true"
aws ssm put-parameter --region us-east-1 --type String \
  --name /waypoint/prod/DEMO_CLOCK_START --value "2026-09-25T15:40:00+05:30"
```

The instance role may read only `/waypoint/prod/*`.

## Usage

```bash
terraform init
terraform fmt -recursive
terraform validate
terraform plan -out=tfplan        # review before applying
terraform apply tfplan            # only after approval
terraform output
```

After apply, set these GitHub repository **variables**:

| Variable | Source |
| --- | --- |
| `AWS_REGION` | `terraform output aws_region` |
| `ECR_REPOSITORY` | `terraform output ecr_repository_url` |
| `EC2_INSTANCE_ID` | `terraform output ec2_instance_id` |
| `API_HEALTH_URL` | `terraform output api_health_url` |
| `AWS_DEPLOY_ROLE_ARN` | `terraform output github_deploy_role_arn` |

## Cost

Roughly **$8–18 / month** while running: EC2 `t3.small` (~$15), 20 GiB gp3
(~$1.6), EIP (free while attached), ECR/S3/SSM (cents). No NAT, ALB, RDS, ECS or
EKS. Terminate the instance when the review window closes to stop the compute
charge.
