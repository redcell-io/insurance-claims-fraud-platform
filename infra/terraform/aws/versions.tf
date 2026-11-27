terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

# Auth via a dedicated AWS CLI profile (named "claimfraud" locally — see
# README.md "Prerequisites"), not the account's default credentials:
# this deployment creates/destroys real paid resources, so it runs as
# its own narrowly-scoped IAM user (AmazonEC2FullAccess only) rather
# than whatever broader personal/admin credentials the default profile
# holds. `aws_profile` has no default — it must be set explicitly in
# terraform.tfvars, so this can never silently fall back to the default
# profile.
provider "aws" {
  region  = var.aws_region
  profile = var.aws_profile

  # Auto-tags every resource that supports it — the one AWS-specific
  # capability neither OCI nor Azure's providers have; see
  # ../shared.tfvars.example's common_tags.
  default_tags {
    tags = var.common_tags
  }
}
