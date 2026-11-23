data "oci_identity_availability_domains" "ads" {
  compartment_id = var.tenancy_ocid
}

# Ampere A1 (ARM) images only come from Canonical Ubuntu on OCI's
# platform-image catalog — pick the newest matching the instance shape,
# which also guarantees aarch64 compatibility for the shape below.
data "oci_core_images" "ubuntu_arm" {
  compartment_id           = var.compartment_ocid
  operating_system         = "Canonical Ubuntu"
  operating_system_version = "22.04"
  shape                    = var.instance_shape
  sort_by                  = "TIMECREATED"
  sort_order               = "DESC"
}

locals {
  # docker-compose.deploy.yml / .deploy.images.yml and the handful of
  # config/ files the deploy stack needs are shipped as cloud-init
  # write_files, not via a git clone on the instance — sidesteps needing
  # any repo credentials (public or private) on the VM entirely, and
  # keeps "what's running" fully pinned to what Terraform applied rather
  # than whatever HEAD happened to be at boot time. See README.md.
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
  })
}

resource "oci_core_instance" "this" {
  compartment_id      = var.compartment_ocid
  availability_domain = data.oci_identity_availability_domains.ads.availability_domains[0].name
  display_name        = "${var.display_name_prefix}-vm"
  shape               = var.instance_shape

  shape_config {
    ocpus         = var.instance_ocpus
    memory_in_gbs = var.instance_memory_in_gbs
  }

  source_details {
    source_type             = "image"
    source_id               = data.oci_core_images.ubuntu_arm.images[0].id
    boot_volume_size_in_gbs = var.boot_volume_size_in_gbs
  }

  create_vnic_details {
    subnet_id              = oci_core_subnet.public.id
    nsg_ids                = [oci_core_network_security_group.instance.id]
    assign_public_ip       = true
    skip_source_dest_check = false
  }

  metadata = {
    ssh_authorized_keys = var.ssh_public_key
    user_data           = base64encode(local.cloud_init)
  }
}

# oci_core_instance doesn't expose the public IP directly — it lives on
# the VNIC attachment, fetched via these two data sources.
data "oci_core_vnic_attachments" "this" {
  compartment_id = var.compartment_ocid
  instance_id    = oci_core_instance.this.id
}

data "oci_core_vnic" "this" {
  vnic_id = data.oci_core_vnic_attachments.this.vnic_attachments[0].vnic_id
}
