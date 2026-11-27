terraform {
  required_version = ">= 1.5.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }
}

# Auth via `az login` (Azure CLI session) — no service-principal secret
# to manage, matches how carefully this repo already treats OCI's own
# API-key file as the one piece of real auth material. Run `az login`
# once before `terraform init`/`plan`; the provider picks up the CLI's
# active session and subscription automatically. `features {}` is
# required by azurerm even when empty.
provider "azurerm" {
  subscription_id = var.subscription_id
  features {}

  # Default behavior auto-registers ~25 resource providers this
  # deployment never touches (BotService, Kusto, DevTestLab, ...),
  # firing them all in parallel — hit "connection may have been reset"
  # on several during a real `terraform plan` against a fresh
  # subscription. This deployment only needs Compute/Network/Resources,
  # which are virtually always already registered on any real
  # subscription; skip the bulk registration pass entirely rather than
  # debug a flaky burst of calls for providers we don't use.
  # (`skip_provider_registration` — not the v4-only
  # `resource_provider_registrations` string — since this is pinned to
  # azurerm ~> 3.0.)
  skip_provider_registration = true
}
