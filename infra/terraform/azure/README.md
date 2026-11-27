# infra/terraform/azure

Provisions a single Azure B2pts v2 VM (2 vCPU/4GB, ARM/Ampere Altra) and
runs the full claim-fraud-platform deploy stack on it — a second path
alongside `infra/terraform/oci/`, added after Oracle's Always Free
Ampere A1 proved persistently out of host capacity (see DECISIONS.md's
entry on this). See that entry for the full rationale.

Scope is deliberately narrow, same as the OCI directory: this
provisions **networking + the VM only**. The application layer is
`docker-compose.deploy.yml` (DECISIONS.md #19), shipped to the instance
via cloud-init rather than a git clone.

**This directory reuses OCIR, not an Azure-native registry.** OCIR is
just a registry reachable over HTTPS — B2pts v2 is ARM, the same
architecture as the `linux/arm64` images `scripts/build-and-push-oci.sh`
already built and pushed for the OCI deploy, so this VM pulls those same
images directly. No Azure Container Registry, no second build pipeline,
no second script. If you ever switch `vm_size` to an x86_64 shape (e.g.
`Standard_B2ats_v2`), you'd need one new `--platform linux/amd64` build
pass through that same script — still no new registry.

## Prerequisites

1. **An Azure account** with a subscription. B-series is a mainstream
   SKU, not a special scarce allocation like OCI's Ampere A1 — but still
   confirm `vm_size` availability in your chosen `location` first:
   `az vm list-skus --location <region> --size Standard_B2pts_v2
   --output table`.
2. **Azure CLI installed, and `az login` run once** — this provider
   authenticates via the CLI's session, not a stored service-principal
   secret. `az account show --query id -o tsv` gives you
   `subscription_id`.

   Two real gotchas hit doing exactly this on Windows, worth avoiding a
   repeat of:
   - **Installing via `winget install -e --id Microsoft.AzureCLI`
     triggers a UAC elevation prompt** — this fails silently (exit code
     1602, "You cancelled the installation") if run from any
     non-interactive context (a script, an agent's tool call, a CI
     step), since there's no one to click "Yes." Run it yourself in an
     interactive terminal and approve the prompt.
   - **A terminal already open before the install won't see `az` on
     `PATH`** — Windows updates the system `PATH` registry value, but an
     already-running shell keeps its own stale copy. Open a **new**
     terminal/tab (or restart your editor) rather than assuming
     `command not found` means the install failed.
   - **`az login --use-device-code` is unreliable when driven
     non-interactively** — repeatedly failed with `AADSTS70016:
     Authorization is pending` even left running for 10+ minutes in a
     background process, never actually completing even when the
     device-code page was reached in a browser. Plain `az login` (no
     device code), run directly in your own interactive terminal, opens
     the normal account/tenant picker and completes immediately —
     that's the one that actually worked. If you're scripting/automating
     this provider's auth at all, don't route it through device-code in
     a non-interactive wrapper; have a human run `az login` once,
     interactively, and let the CLI's cached session carry forward.
3. **The same SSH keypair and OCIR credentials already set up for
   `infra/terraform/oci`** — both live in `../shared.tfvars`, not
   duplicated here. If you haven't done the OCI side at all, see that
   directory's own README "Prerequisites" for where the OCIR values come
   from (`ocir_region_key`/`ocir_namespace`/`ocir_username`/
   `ocir_auth_token`) — Azure needs the exact same ones.
4. **`terraform`** (already on `PATH` in this repo).

## Usage

```sh
cd infra/terraform/azure
cp ../shared.tfvars.example ../shared.tfvars   # if you haven't already — shared across clouds
cp terraform.tfvars.example terraform.tfvars   # Azure-specific values; gitignored
az login
terraform init
terraform plan -var-file=../shared.tfvars -var-file=terraform.tfvars    # review before applying — this creates real, billable resources
terraform apply -var-file=../shared.tfvars -var-file=terraform.tfvars
```

If the images aren't already on OCIR (first time deploying to *either*
cloud), build and push them from the repo root first — see
`infra/terraform/oci/README.md`'s "Usage" section for the exact
`proto/generate.sh` + `scripts/build-and-push-oci.sh` commands. Nothing
here is OCI-specific about that step; it's just where the images happen
to live.

`terraform output` prints the instance's public IP, a ready-to-paste
`ssh_command`, and reminds you of the REST/Prometheus URLs. Give
cloud-init a minute or two after `apply` finishes — `ssh` in and `sudo
tail -f /var/log/cloud-init-output.log` to watch it install Docker, log
into OCIR, and bring the stack up.

**Verify** with TC-01 from `docs/manual-test-cases.md`, against the
instance's public IP instead of `localhost`:

```sh
curl -s -X POST http://<instance_public_ip>:8080/v1/claims \
  -H "Content-Type: application/json" \
  -d '{"claim_id":"clm-azure-1","product":"auto","event_type":"claim.fnol","policy_number":"POL-123456","claimant_name":"Jane Doe","raw_address":"123 Main St, Springfield, IL 62704"}'
```

## Tearing down

```sh
terraform destroy -var-file=../shared.tfvars -var-file=terraform.tfvars
```

## What's deliberately not here (demo-grade, same framing as the OCI directory)

- **No TLS, no domain** — the REST API and Prometheus UI are plain HTTP
  on a bare public IP.
- **No remote Terraform state** — `terraform.tfstate` is local and
  gitignored. Fine solo; would need an Azure Storage Account backend if
  more than one machine ever applies this.
- **No managed-identity auth for OCIR** — the auth token sits in
  plaintext in the rendered cloud-init (readable from the instance's own
  metadata service), same accepted tradeoff as the OCI directory. Not
  something Azure managed identity would even help with here, since the
  registry being authenticated against (OCIR) isn't Azure's own.
- **No healthchecks on `depends_on`** — same race the OCI directory
  documents: a claim submitted seconds after boot can hit
  `DeadlineExceeded` while the Java services are still starting. Give it
  a minute.
- **No CI** — `scripts/build-and-push-oci.sh` is run by hand.

## If B2pts v2 doesn't work out

Unlike OCI's Ampere A1, B-series availability issues would be
surprising — but if `vm_size = "Standard_B2pts_v2"` can't be provisioned
in your region, the fallback is `Standard_B2ats_v2` (x86_64, also
burstable, similar price/spec). That needs a new image build:
`OCIR_REGION=... OCIR_NAMESPACE=... IMAGE_TAG=latest-amd64 ./scripts/
build-and-push-oci.sh` with `--platform linux/arm64` changed to
`linux/amd64` in that script (or a small copy of it) — still the same
OCIR registry, still no Azure Container Registry.
