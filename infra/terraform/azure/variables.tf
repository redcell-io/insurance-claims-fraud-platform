# --- Azure auth/placement ---

variable "subscription_id" {
  type        = string
  description = "Azure subscription to deploy into. `az account show` shows the CLI's active one — explicit here rather than relying on it silently, even though azurerm would otherwise infer it from the az login session."
}

variable "resource_group_name" {
  type    = string
  default = "claimfraud-rg"
}

variable "location" {
  type        = string
  description = "Azure region, e.g. eastus. Check B-series availability for your chosen vm_size there first (az vm list-skus --location <region> --size Standard_B2pts_v2 --output table)."
}

# --- Instance access — same names/meaning as infra/terraform/oci's own
# copies; the actual values come from ../shared.tfvars, not here. ---

variable "ssh_public_key" {
  type        = string
  description = "Public key (contents, not a path) installed for the azureuser account via the VM's admin_ssh_key."
}

variable "ssh_allowed_cidr" {
  type        = string
  description = "CIDR allowed to SSH in (port 22). Restrict this to your own IP/32 — never leave it 0.0.0.0/0."
}

variable "rest_allowed_cidr" {
  type        = string
  description = "CIDR allowed to reach the claims-gateway REST API (:8080). 0.0.0.0/0 is the point of a public demo, but narrow it if this is ever more than a throwaway."
  default     = "0.0.0.0/0"
}

variable "prometheus_allowed_cidr" {
  type        = string
  description = "CIDR allowed to reach the Prometheus UI (:9090). No auth on that UI at all, so — unlike rest_allowed_cidr — this defaults to being wide open too, but should usually be narrowed to your own IP; widen it back temporarily only when you actually want to share a live demo link."
  default     = "0.0.0.0/0"
}

variable "display_name_prefix" {
  type    = string
  default = "claimfraud"
}

variable "common_tags" {
  type        = map(string)
  default     = {}
  description = "Applied via an explicit tags argument on each resource that supports it — azurerm has no provider-level auto-tag mechanism (unlike AWS's default_tags). See ../shared.tfvars.example."
}

# --- VM shape ---

variable "vm_size" {
  type        = string
  default     = "Standard_B2pts_v2"
  description = "ARM (Ampere Altra) burstable size — matches the linux/arm64 images already built and pushed to OCIR by scripts/build-and-push-oci.sh, so this VM needs no separate image build or registry. Unlike OCI's Always Free Ampere A1, B-series is a mainstream, non-scarce SKU. Fallback if this size has its own availability issues: Standard_B2ats_v2 (x86_64) — would need a new `--platform linux/amd64` build pass through the same script (faster than the arm64 build, native to most dev machines), still no new registry."
}

variable "vnet_cidr" {
  type    = string
  default = "10.0.0.0/16"
}

variable "subnet_cidr" {
  type    = string
  default = "10.0.1.0/24"
}

# --- Container registry (OCIR, reused — not Azure's own) — see
# scripts/build-and-push-oci.sh and infra/terraform/oci/README.md
# "Porting to another cloud". Same names/meaning as OCI's own copies;
# values come from ../shared.tfvars since both clouds pull from the same
# registry. ---

variable "ocir_region_key" {
  type        = string
  description = "OCIR region key for the image registry host, e.g. iad (us-ashburn-1), ord (us-chicago-1). Forms <ocir_region_key>.ocir.io."
}

variable "ocir_namespace" {
  type        = string
  description = "Tenancy's object storage namespace (OCIR images live at <region>.ocir.io/<namespace>/...)."
}

variable "ocir_username" {
  type        = string
  description = "OCIR login username, formatted <namespace>/<oci-username> (or <namespace>/oracleidentitycloudservice/<username> for federated/IDCS users)."
}

variable "ocir_auth_token" {
  type        = string
  sensitive   = true
  description = "OCI auth token used as the OCIR login password. Embedded in plaintext into the instance's custom_data — same accepted demo-grade tradeoff as OCI's own copy of this variable."
}
