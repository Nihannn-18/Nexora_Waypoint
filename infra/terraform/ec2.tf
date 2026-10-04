resource "aws_eip" "waypoint" {
  domain = "vpc"

  tags = merge(local.tags, { Name = "${local.name_prefix}-api" })
}

resource "aws_instance" "api" {
  ami                    = data.aws_ami.al2023.id
  instance_type          = var.instance_type
  subnet_id              = data.aws_subnet.selected.id
  iam_instance_profile   = aws_iam_instance_profile.instance.name
  vpc_security_group_ids = [aws_security_group.api.id]

  # Deliberately false: a change to the bootstrap script should not silently
  # recreate a running instance. Re-run bootstrap through SSM instead.
  user_data_replace_on_change = false

  user_data = templatefile("${path.module}/templates/user-data.sh.tftpl", {
    compose_version   = var.compose_version
    compose_b64       = base64encode(local.compose_file)
    caddy_b64         = base64encode(local.caddy_file)
    render_env_b64    = base64encode(local.render_env_script)
    deploy_script_b64 = base64encode(local.deploy_script)
    service_b64       = base64encode(local.service_file)
    config_env_b64    = base64encode(local.config_env)
  })

  root_block_device {
    volume_type           = "gp3"
    volume_size           = var.root_volume_size_gb
    encrypted             = true
    delete_on_termination = true
  }

  # IMDSv2 only.
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  tags = merge(local.tags, { Name = "${local.name_prefix}-api" })
}

resource "aws_eip_association" "waypoint" {
  instance_id   = aws_instance.api.id
  allocation_id = aws_eip.waypoint.id
}
