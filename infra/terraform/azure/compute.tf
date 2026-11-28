locals {
  # Mirrors infra/terraform/oci/compute.tf's own locals exactly — same
  # repo_root, same file list, same reasoning (shipped via cloud-init
  # write_files, not a git clone, so no repo credentials ever need to
  # exist on the VM; "what's running" is pinned to what Terraform
  # applied). cloud-init.yaml.tftpl itself is copied verbatim from the
  # OCI directory — it's not OCI-specific, it only references
  # deploy_files/ocir_* template variables.
  repo_root = "${path.module}/../../.."

  deploy_files = {
    # file(), not just the path string — a real bug found live on a
    # first AWS apply of this exact pattern: without file(), each
    # "content" below is the bare path string itself, which then gets
    # base64-encoded and written to the instance as that file's
    # "content" (e.g. docker-compose.deploy.yml on the VM literally
    # contained the text "./../../../docker-compose.deploy.yml").
    # docker compose then failed trying to parse that string as YAML
    # (`yaml: construct errors: ... cannot construct !!str`).
    "docker-compose.deploy.yml"                      = file("${local.repo_root}/docker-compose.deploy.yml")
    "docker-compose.deploy.images.yml"               = file("${local.repo_root}/docker-compose.deploy.images.yml")
    "config/tenants.yaml"                            = file("${local.repo_root}/config/tenants.yaml")
    "config/prometheus.docker.yml"                   = file("${local.repo_root}/config/prometheus.docker.yml")
    "config/dag/acme_insurance/auto/claim.fnol.yaml" = file("${local.repo_root}/config/dag/acme_insurance/auto/claim.fnol.yaml")
  }

  cloud_init = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    deploy_files    = local.deploy_files
    ocir_host       = "${var.ocir_region_key}.ocir.io"
    ocir_region_key = var.ocir_region_key
    ocir_namespace  = var.ocir_namespace
    ocir_username   = var.ocir_username
    ocir_auth_token = var.ocir_auth_token
    # "latest" — this VM is arm64, same architecture/tag as the images
    # scripts/build-and-push-oci.sh already pushes by default. Only
    # infra/terraform/aws (x86_64) needs a different value here.
    image_tag = "latest"
  })
}

resource "azurerm_network_interface" "this" {
  name                = "${var.display_name_prefix}-nic"
  location            = azurerm_resource_group.this.location
  resource_group_name = azurerm_resource_group.this.name
  tags                = var.common_tags

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.public.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.this.id
  }
}

resource "azurerm_network_interface_security_group_association" "this" {
  network_interface_id      = azurerm_network_interface.this.id
  network_security_group_id = azurerm_network_security_group.instance.id
}

resource "azurerm_linux_virtual_machine" "this" {
  name                = "${var.display_name_prefix}-vm"
  resource_group_name = azurerm_resource_group.this.name
  location            = azurerm_resource_group.this.location
  size                = var.vm_size
  tags                = merge(var.common_tags, { Name = "${var.display_name_prefix}-vm" })
  # "ubuntu", not Azure's more common "azureuser" — lets
  # cloud-init.yaml.tftpl be reused byte-for-byte from
  # infra/terraform/oci (it hardcodes `usermod -aG docker ubuntu`,
  # matching OCI's Canonical-image default user).
  admin_username = "ubuntu"

  network_interface_ids = [azurerm_network_interface.this.id]

  admin_ssh_key {
    username   = "ubuntu"
    public_key = var.ssh_public_key
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }

  # Ubuntu 22.04 LTS, arm64 — matches OCI's own image choice
  # (infra/terraform/oci/compute.tf) for consistency. Unverified against
  # a real account until the first `terraform plan`/`apply` — confirm
  # with `az vm image list --location <location> --publisher Canonical
  # --architecture Arm64 --all --output table` if this exact sku isn't
  # found, same caution applied to OCI's own image data source.
  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts-arm64"
    version   = "latest"
  }

  custom_data = base64encode(local.cloud_init)
}
