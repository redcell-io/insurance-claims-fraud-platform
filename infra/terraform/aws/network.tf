resource "aws_vpc" "this" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = "${var.display_name_prefix}-vpc" }
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id

  tags = { Name = "${var.display_name_prefix}-igw" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.this.id
  }

  tags = { Name = "${var.display_name_prefix}-public-rt" }
}

resource "aws_subnet" "public" {
  vpc_id            = aws_vpc.this.id
  cidr_block        = var.subnet_cidr
  availability_zone = var.availability_zone
  # Belt-and-braces — the instance's own associate_public_ip_address is
  # what actually matters (see compute.tf), but this keeps the subnet
  # itself consistent with being a "public" one.
  map_public_ip_on_launch = true

  tags = { Name = "${var.display_name_prefix}-public-subnet" }
}

resource "aws_route_table_association" "public" {
  subnet_id      = aws_subnet.public.id
  route_table_id = aws_route_table.public.id
}

# Unlike Azure's NSGs (which ship a built-in allow-all-outbound default)
# and like OCI's NSGs, a custom aws_security_group with no egress block
# has ZERO outbound rules — AWS does not carry over the default VPC
# security group's allow-all-egress behavior to one you create yourself.
# Confirmed via a dedicated validation pass before writing this (real
# paid resources, no room for a wrong guess here) — see DECISIONS.md.
resource "aws_security_group" "instance" {
  name        = "${var.display_name_prefix}-sg"
  description = "claim-fraud-platform demo instance"
  vpc_id      = aws_vpc.this.id

  # SSH — restrict ssh_allowed_cidr to your own IP/32 in terraform.tfvars.
  ingress {
    description = "ssh"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.ssh_allowed_cidr]
  }

  # claims-gateway REST API — the one endpoint the demo is actually about.
  ingress {
    description = "rest"
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = [var.rest_allowed_cidr]
  }

  # Prometheus UI — so the metrics pass (DECISIONS.md #18) is actually
  # viewable on the deployed instance, not just locally.
  ingress {
    description = "prometheus"
    from_port   = 9090
    to_port     = 9090
    protocol    = "tcp"
    cidr_blocks = [var.prometheus_allowed_cidr]
  }

  # Deliberately no public rule for the gRPC (909x) or per-service
  # metrics (919x) ports — same reasoning as the other two clouds: those
  # stay container-internal, reachable via an SSH tunnel for debugging.

  # Required for outbound internet access at all (docker login, image
  # pulls) — see this resource's own comment above. Not a convenience,
  # a hard requirement.
  egress {
    description = "allow all outbound"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${var.display_name_prefix}-sg" }
}
