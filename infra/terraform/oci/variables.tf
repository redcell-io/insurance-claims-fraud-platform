# OCI API auth — see README.md "Prerequisites" for how to generate
# these (OCI console: Profile -> My profile -> API keys -> Add API key).
variable "tenancy_ocid" {
  type        = string
  description = "OCID of the OCI tenancy."
}

variable "user_ocid" {
  type        = string
  description = "OCID of the OCI user whose API key is used."
}

variable "fingerprint" {
  type        = string
  description = "Fingerprint of the uploaded API signing key."
}

variable "private_key_path" {
  type        = string
  description = "Local path to the API signing key's PEM private key."
}

variable "region" {
  type        = string
  description = "OCI region, e.g. us-ashburn-1."
}

variable "compartment_ocid" {
  type        = string
  description = "OCID of the compartment to create resources in."
}

# --- Instance access ---

variable "ssh_public_key" {
  type        = string
  description = "Public key (contents, not a path) installed for the opc/ubuntu user via cloud-init."
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

# --- Shape (defaults are the OCI Always Free Ampere A1 allotment) ---

variable "instance_shape" {
  type        = string
  default     = "VM.Standard.A1.Flex"
  description = "Always Free eligible on OCI: up to 4 OCPU / 24GB total across all A1.Flex instances in a tenancy."
}

variable "instance_ocpus" {
  type    = number
  default = 4
}

variable "instance_memory_in_gbs" {
  type    = number
  default = 24
}

variable "boot_volume_size_in_gbs" {
  type        = number
  default     = 50
  description = "50GB is within the Always Free block-volume allotment (200GB total)."
}

# --- Networking ---

variable "vcn_cidr" {
  type    = string
  default = "10.0.0.0/16"
}

variable "subnet_cidr" {
  type    = string
  default = "10.0.1.0/24"
}

variable "display_name_prefix" {
  type    = string
  default = "claimfraud"
}

# --- Container registry (OCIR) — see scripts/build-and-push-oci.sh ---

variable "ocir_region_key" {
  type        = string
  description = "OCIR region key for the image registry host, e.g. iad (us-ashburn-1), phx (us-phoenix-1) — see https://docs.oracle.com/iaas/Content/Registry/Concepts/registryprerequisites.htm#regional-availability. Forms <ocir_region_key>.ocir.io."
}

variable "ocir_namespace" {
  type        = string
  description = "Tenancy's object storage namespace (OCIR images live at <region>.ocir.io/<namespace>/...). Find via `oci os ns get`."
}

variable "ocir_username" {
  type        = string
  description = "OCIR login username, formatted <namespace>/<oci-username> (or <namespace>/oracleidentitycloudservice/<username> for federated/IDCS users)."
}

variable "ocir_auth_token" {
  type        = string
  sensitive   = true
  description = "OCI auth token (My profile -> Auth tokens -> Generate token) used as the OCIR login password. Embedded in plaintext into the instance's cloud-init user_data — acceptable for this demo-grade deploy (DECISIONS.md #21), but don't reuse a token that guards anything more sensitive. A production version would use instance-principal auth (a dynamic group + policy) instead of a static token."
}
