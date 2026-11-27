# Canonical's official publisher account — confirmed current via a
# dedicated validation pass (real paid resources, no room for a wrong
# guess). root-device-type=ebs is cheap insurance: t3.medium is
# EBS-only, and Canonical has occasionally published instance-store
# variants under similar name prefixes.
data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"]

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  filter {
    name   = "root-device-type"
    values = ["ebs"]
  }
}

locals {
  # Mirrors infra/terraform/oci's and infra/terraform/azure's own locals
  # exactly — same repo_root, same file list, same reasoning (shipped
  # via cloud-init write_files, not a git clone, so no repo credentials
  # ever need to exist on the VM). cloud-init.yaml.tftpl itself is
  # copied verbatim from the OCI directory.
  repo_root = "${path.module}/../../.."

  deploy_files = {
    "docker-compose.deploy.yml"                      = "${local.repo_root}/docker-compose.deploy.yml"
    "docker-compose.deploy.images.yml"               = "${local.repo_root}/docker-compose.deploy.images.yml"
    "config/tenants.yaml"                            = "${local.repo_root}/config/tenants.yaml"
    "config/prometheus.docker.yml"                   = "${local.repo_root}/config/prometheus.docker.yml"
    "config/dag/acme_insurance/auto/claim.fnol.yaml" = "${local.repo_root}/config/dag/acme_insurance/auto/claim.fnol.yaml"
  }

  cloud_init = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    deploy_files    = local.deploy_files
    ocir_host       = "${var.ocir_region_key}.ocir.io"
    ocir_region_key = var.ocir_region_key
    ocir_namespace  = var.ocir_namespace
    ocir_username   = var.ocir_username
    ocir_auth_token = var.ocir_auth_token
    # This VM is x86_64, not arm64 like OCI's/Azure's — pulls the
    # separate amd64 image set pushed via `PLATFORM=linux/amd64
    # IMAGE_TAG=latest-amd64 scripts/build-and-push-oci.sh` (see that
    # script's own usage comment and DECISIONS.md).
    image_tag = "latest-amd64"
  })
}

# AWS has no OCI/Azure-style "pass a key directly as instance metadata"
# mechanism — confirmed via a dedicated validation pass. A key pair
# resource is the only native path; var.ssh_public_key is already a
# string (from ../shared.tfvars), no file needed.
resource "aws_key_pair" "this" {
  key_name   = "${var.display_name_prefix}-key"
  public_key = var.ssh_public_key
}

resource "aws_instance" "this" {
  ami                    = data.aws_ami.ubuntu.id
  instance_type          = var.instance_type
  key_name               = aws_key_pair.this.key_name
  subnet_id              = aws_subnet.public.id
  vpc_security_group_ids = [aws_security_group.instance.id]

  # No aws_eip needed for a single ad-hoc instance meant to be torn down
  # right after verification — confirmed this is sufficient (combined
  # with the subnet's route to the internet gateway) via the same
  # validation pass, and it avoids the idle-EIP-cost risk a forgotten
  # Elastic IP would introduce for something deliberately short-lived.
  associate_public_ip_address = true

  # Plain text — Terraform/AWS base64-encodes this internally.
  # Confirmed via a dedicated validation pass: unlike OCI's
  # metadata.user_data and Azure's custom_data (both of which require
  # base64encode() explicitly), wrapping this in base64encode() here
  # would double-encode it and break cloud-init parsing.
  user_data = local.cloud_init

  tags = { Name = "${var.display_name_prefix}-vm" }
}
