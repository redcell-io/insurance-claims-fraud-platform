resource "azurerm_resource_group" "this" {
  name     = var.resource_group_name
  location = var.location
  tags     = var.common_tags
}

resource "azurerm_virtual_network" "this" {
  name                = "${var.display_name_prefix}-vnet"
  address_space       = [var.vnet_cidr]
  location            = azurerm_resource_group.this.location
  resource_group_name = azurerm_resource_group.this.name
  tags                = var.common_tags
}

# azurerm_subnet has no tags argument at all — Azure subnets aren't
# independently taggable, unlike every other resource here.
resource "azurerm_subnet" "public" {
  name                 = "${var.display_name_prefix}-public-subnet"
  resource_group_name  = azurerm_resource_group.this.name
  virtual_network_name = azurerm_virtual_network.this.name
  address_prefixes     = [var.subnet_cidr]
}

resource "azurerm_public_ip" "this" {
  name                = "${var.display_name_prefix}-public-ip"
  resource_group_name = azurerm_resource_group.this.name
  location            = azurerm_resource_group.this.location
  allocation_method   = "Static"
  sku                 = "Standard"
  tags                = var.common_tags
}

# Unlike OCI's NSGs, Azure NSGs ship with built-in default rules
# (including AllowInternetOutBound) — no explicit egress rule needed
# here, one fewer resource than infra/terraform/oci/network.tf.
resource "azurerm_network_security_group" "instance" {
  name                = "${var.display_name_prefix}-nsg"
  location            = azurerm_resource_group.this.location
  resource_group_name = azurerm_resource_group.this.name
  tags                = var.common_tags
}

# SSH — restrict ssh_allowed_cidr to your own IP/32 in terraform.tfvars.
resource "azurerm_network_security_rule" "ssh_ingress" {
  name                        = "ssh-ingress"
  priority                    = 100
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_range      = "22"
  source_address_prefix       = var.ssh_allowed_cidr
  destination_address_prefix  = "*"
  resource_group_name         = azurerm_resource_group.this.name
  network_security_group_name = azurerm_network_security_group.instance.name
}

# claims-gateway REST API — the one endpoint the demo is actually about.
resource "azurerm_network_security_rule" "rest_ingress" {
  name                        = "rest-ingress"
  priority                    = 110
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_range      = "8080"
  source_address_prefix       = var.rest_allowed_cidr
  destination_address_prefix  = "*"
  resource_group_name         = azurerm_resource_group.this.name
  network_security_group_name = azurerm_network_security_group.instance.name
}

# Prometheus UI — so the metrics pass (DECISIONS.md #18) is actually
# viewable on the deployed instance, not just locally.
resource "azurerm_network_security_rule" "prometheus_ingress" {
  name                        = "prometheus-ingress"
  priority                    = 120
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_range      = "9090"
  source_address_prefix       = var.prometheus_allowed_cidr
  destination_address_prefix  = "*"
  resource_group_name         = azurerm_resource_group.this.name
  network_security_group_name = azurerm_network_security_group.instance.name
}

# Deliberately no public rule for the gRPC (909x) or per-service metrics
# (919x) ports — same reasoning as infra/terraform/oci/network.tf: those
# stay container-internal, reachable via an SSH tunnel for debugging.
#
# None of the azurerm_network_security_rule resources above take tags
# either — same sub-resource limitation as OCI's NSG security rules.
