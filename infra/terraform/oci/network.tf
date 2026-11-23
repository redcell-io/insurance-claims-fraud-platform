resource "oci_core_vcn" "this" {
  compartment_id = var.compartment_ocid
  cidr_blocks    = [var.vcn_cidr]
  display_name   = "${var.display_name_prefix}-vcn"
  dns_label      = "claimfraud"
}

resource "oci_core_internet_gateway" "this" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.this.id
  display_name   = "${var.display_name_prefix}-igw"
  enabled        = true
}

# The VCN's auto-created default route table — edited in place rather
# than replaced with a separate oci_core_route_table, since this VCN
# only ever needs the one route (a single public subnet, no NAT/private
# tier — demo-grade, per DECISIONS.md #19/#21).
resource "oci_core_default_route_table" "this" {
  manage_default_resource_id = oci_core_vcn.this.default_route_table_id

  route_rules {
    destination       = "0.0.0.0/0"
    destination_type  = "CIDR_BLOCK"
    network_entity_id = oci_core_internet_gateway.this.id
  }
}

resource "oci_core_subnet" "public" {
  compartment_id             = var.compartment_ocid
  vcn_id                     = oci_core_vcn.this.id
  cidr_block                 = var.subnet_cidr
  display_name               = "${var.display_name_prefix}-public-subnet"
  dns_label                  = "public"
  route_table_id             = oci_core_default_route_table.this.id
  prohibit_public_ip_on_vnic = false
}

resource "oci_core_network_security_group" "instance" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.this.id
  display_name   = "${var.display_name_prefix}-nsg"
}

# SSH — restrict ssh_allowed_cidr to your own IP/32 in terraform.tfvars.
resource "oci_core_network_security_group_security_rule" "ssh_ingress" {
  network_security_group_id = oci_core_network_security_group.instance.id
  direction                 = "INGRESS"
  protocol                  = "6" # TCP
  source                    = var.ssh_allowed_cidr
  source_type               = "CIDR_BLOCK"
  tcp_options {
    destination_port_range {
      min = 22
      max = 22
    }
  }
}

# claims-gateway REST API — the one endpoint the demo is actually about.
resource "oci_core_network_security_group_security_rule" "rest_ingress" {
  network_security_group_id = oci_core_network_security_group.instance.id
  direction                 = "INGRESS"
  protocol                  = "6"
  source                    = var.demo_allowed_cidr
  source_type               = "CIDR_BLOCK"
  tcp_options {
    destination_port_range {
      min = 8080
      max = 8080
    }
  }
}

# Prometheus UI — so the metrics pass (DECISIONS.md #18) is actually
# viewable on the deployed instance, not just locally.
resource "oci_core_network_security_group_security_rule" "prometheus_ingress" {
  network_security_group_id = oci_core_network_security_group.instance.id
  direction                 = "INGRESS"
  protocol                  = "6"
  source                    = var.demo_allowed_cidr
  source_type               = "CIDR_BLOCK"
  tcp_options {
    destination_port_range {
      min = 9090
      max = 9090
    }
  }
}

# Deliberately no public ingress for the gRPC (909x) or per-service
# metrics (919x) ports — those are container-to-container only in
# docker-compose.deploy.yml. Reach them for debugging via an SSH tunnel
# (`ssh -L 9092:localhost:9092 ...`), not a public NSG rule.

resource "oci_core_network_security_group_security_rule" "egress_all" {
  network_security_group_id = oci_core_network_security_group.instance.id
  direction                 = "EGRESS"
  protocol                  = "all"
  destination               = "0.0.0.0/0"
  destination_type          = "CIDR_BLOCK"
}
