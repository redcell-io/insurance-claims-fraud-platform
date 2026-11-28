# --- AWS auth/placement ---

variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "aws_profile" {
  type        = string
  description = "Named AWS CLI profile to authenticate as (see README.md \"Prerequisites\") — no default, must be set explicitly so this can never silently fall back to the account's default credentials."
}

# --- Instance access — same names/meaning as infra/terraform/oci's and
# infra/terraform/azure's own copies; the actual values come from
# ../shared.tfvars, not here. ---

variable "ssh_public_key" {
  type        = string
  description = "Public key (contents, not a path). AWS has no direct metadata-injection mechanism like OCI/Azure — this gets wrapped in an aws_key_pair resource instead (see compute.tf)."
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
  type        = string
  description = "../shared.tfvars sets this to \"claimfraud\" for OCI/Azure (OCI already has 9 live resources named claimfraud-*, renaming would force recreation) — this stack has nothing created yet, so terraform.tfvars overrides it to \"claimsfraud\" here specifically. No default: must come from one of the two tfvars files, not silently fall back to either spelling."
}

variable "common_tags" {
  type        = map(string)
  default     = {}
  description = "Applied automatically to every taggable resource via the provider's default_tags block (versions.tf) — unlike OCI/Azure, no per-resource tags argument needed. See ../shared.tfvars.example."
}

# --- Instance shape ---

variable "instance_type" {
  type        = string
  default     = "t3.medium"
  description = "x86_64, 4GB — deliberately NOT the free-tier t2.micro/t3.micro (1GB), which carries the same resource-starvation risk already rejected for GCP's e2-micro at the start of this effort. This is a real, paid instance; verify and tear down promptly (scripts/teardown-cloud.sh aws)."
}

variable "vpc_cidr" {
  type    = string
  default = "10.0.0.0/16"
}

variable "subnet_cidr" {
  type    = string
  default = "10.0.1.0/24"
}

variable "availability_zone" {
  type        = string
  default     = "us-east-1a"
  description = "Pinned explicitly, not left to AWS's default placement — a real terraform apply landed the subnet in us-east-1e, which doesn't support t3.medium (\"Unsupported: ... not supported in your requested Availability Zone\"; the error listed us-east-1a/b/c/d/f as working). Not every AZ in a region supports every instance type; us-east-1e specifically has a history of more restricted support. If you change aws_region, pick a matching AZ for it too."
}

# --- Container registry (OCIR, reused — not AWS's own) — see
# scripts/build-and-push-oci.sh and infra/terraform/oci/README.md
# "Porting to another cloud". Same names/meaning as the other two
# clouds' copies; values come from ../shared.tfvars. ---

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
  description = "OCI auth token used as the OCIR login password. Embedded in plaintext into the instance's user_data — same accepted demo-grade tradeoff as the other two clouds' copy of this variable."
}
