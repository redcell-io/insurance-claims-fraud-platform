# infra/terraform/oci

Provisions a single OCI Always Free Ampere A1 VM (4 OCPU/24GB ARM) and
runs the full claim-fraud-platform deploy stack on it — the demo-grade
VM deployment DECISIONS.md #19 containerized the stack for. See
DECISIONS.md #21 for the full rationale (why Terraform, why OCI, why a
container-registry image pipeline instead of building on the VM).

Scope is deliberately narrow: this provisions **networking + the VM
only**. The application layer is `docker-compose.deploy.yml` (already
built, DECISIONS.md #19) running on top, shipped to the instance via
cloud-init `write_files` rather than a git clone — see `compute.tf`'s
`local.deploy_files` comment for why.

## Prerequisites

1. **An OCI account** with Always Free resources available in some
   region (not all regions have A1 capacity free-tier-eligible —
   check before picking `region`).
2. **An API signing key**, for Terraform's own auth (separate from SSH):
   OCI Console -> Profile icon -> *My profile* -> *API keys* -> *Add API
   key* -> download the private key, note the fingerprint shown. Gives
   you `user_ocid`, `fingerprint`, `private_key_path` (wherever you save
   the downloaded `.pem`), and `tenancy_ocid` (shown on the same add-key
   dialog, or Profile -> Tenancy).
3. **An OCIR auth token**, for pushing/pulling container images (OCIR
   doesn't accept the API signing key for Docker auth): same *My
   profile* page -> *Auth tokens* -> *Generate token*. This is
   `ocir_auth_token`. `ocir_username` is
   `<namespace>/<your-oci-username>` (or
   `<namespace>/oracleidentitycloudservice/<username>` if you log in via
   IDCS/federated SSO) — the console's OCIR "Getting Started" page shows
   your exact username format. `ocir_namespace` is your tenancy's object
   storage namespace: `oci os ns get` (OCI CLI) or Console -> *Storage* ->
   *Buckets* -> look at any bucket's namespace.
4. **An SSH keypair** — `ssh_public_key` is the public half's contents
   (not a path).
5. **`terraform`** (this repo already has it on `PATH` locally) and
   **`docker buildx`** (ships with Docker Desktop) for
   `scripts/build-and-push-oci.sh`.

## Usage

Values genuinely identical across every cloud this repo deploys to (ssh
key, CIDRs, OCIR creds) live one level up in `../shared.tfvars`, not
here — see `infra/terraform/shared.tfvars.example`. Both files are
needed on every `plan`/`apply`:

```sh
cd infra/terraform/oci
cp ../shared.tfvars.example ../shared.tfvars   # if you haven't already — shared across clouds
cp terraform.tfvars.example terraform.tfvars   # OCI-specific values; gitignored
terraform init
terraform plan -var-file=../shared.tfvars -var-file=terraform.tfvars    # review before applying — this creates real, billable-adjacent resources
terraform apply -var-file=../shared.tfvars -var-file=terraform.tfvars
```

Then, from the repo root, build and push the 7 service images (must
happen before or the instance's first `docker compose pull` fails —
rerun it on the instance via SSH if images weren't ready yet: `cd
/opt/claimfraud && docker compose -f docker-compose.deploy.yml -f
docker-compose.deploy.images.yml pull && docker compose ... up -d`):

```sh
cd ../../..   # repo root
proto/generate.sh   # proto/gen/go must exist before building — see DECISIONS.md #19
OCIR_REGION=iad OCIR_NAMESPACE=yourtenancynamespace \
  OCIR_USERNAME=yourtenancynamespace/you@example.com OCIR_AUTH_TOKEN=... \
  ./scripts/build-and-push-oci.sh
```

`terraform output` prints the instance's public IP, a ready-to-paste
`ssh_command`, and reminds you of the REST/Prometheus URLs. Give
cloud-init a minute or two after `apply` finishes before expecting the
stack to answer — `ssh` in and `sudo tail -f
/var/log/cloud-init-output.log` to watch it install Docker, log into
OCIR, and bring the stack up.

**Verify** with TC-01 from `docs/manual-test-cases.md`, against the
instance's public IP instead of `localhost`:

```sh
curl -s -X POST http://<instance_public_ip>:8080/v1/claims \
  -H "Content-Type: application/json" \
  -d '{"claim_id":"clm-oci-1","product":"auto","event_type":"claim.fnol","policy_number":"POL-123456","claimant_name":"Jane Doe","raw_address":"123 Main St, Springfield, IL 62704"}'
```

## Tearing down

```sh
terraform destroy -var-file=../shared.tfvars -var-file=terraform.tfvars
```

This is the whole point of doing this in Terraform rather than clicking
through the OCI console — one command, nothing left running, nothing
left to forget about.

## What's deliberately not here (demo-grade, same framing as DECISIONS.md #19)

- **No TLS, no domain** — the REST API and Prometheus UI are plain HTTP
  on a bare public IP.
- **No remote Terraform state** — `terraform.tfstate` is local and
  gitignored. Fine solo; would need an S3/OCI-Object-Storage backend if
  more than one machine ever applies this.
- **No instance-principal auth for OCIR** — the auth token in
  `ocir_auth_token` sits in plaintext in the rendered cloud-init
  (readable from the instance's own metadata service). A production
  version would grant the VM a dynamic group + policy instead of
  embedding a static token.
- **No healthchecks on `depends_on`** — same race DECISIONS.md #19
  already documents for the containerized stack: a claim submitted
  seconds after boot can hit `DeadlineExceeded` while the Java services
  are still starting. Give it a minute.
- **No CI** — `scripts/build-and-push-oci.sh` is run by hand. A GitHub
  Actions workflow doing this on every push to `main` is the obvious
  next step, not done here.

## Porting to another cloud

Don't expect this directory's `.tf` files to port — OCI's resource
model (compartments, VCNs, NSGs) and another cloud's (AWS's VPC/EC2
security groups, Azure's resource groups/VNets) are different enough
that a new `infra/terraform/<cloud>/` is a rewrite of the resource
blocks, not a find-and-replace. What *does* carry over: the cloud-init
`write_files` approach, the `terraform plan`/`apply`/`destroy` workflow,
and the shared-tfvars split (`../shared.tfvars`) for values that are
genuinely identical regardless of provider.

**`infra/terraform/azure/` is a real example, not a hypothetical** —
and it carries over more than expected: rather than swapping OCIR for a
same-shaped registry (e.g. ECR), it reuses OCIR *unchanged*. OCIR is
just a registry reachable over HTTPS; Azure's B2pts v2 (ARM, same
architecture as the images already built here) pulls the exact same
`ord.ocir.io/...` images this directory's VM does, via the same
`docker-compose.deploy.images.yml` and the same
`scripts/build-and-push-oci.sh` — no Azure Container Registry, no second
build pipeline. A cloud whose compute architecture doesn't match (e.g.
an x86_64-only shape) would need a second `--platform linux/amd64` build
pass through that same script, but still no new registry.

**`infra/terraform/aws/` confirms the x86_64 case directly** — its
`t3.medium` isn't arm64, so it genuinely does need that second build
pass (`PLATFORM=linux/amd64 IMAGE_TAG=latest-amd64
scripts/build-and-push-oci.sh`, pushed under a distinct tag so it
doesn't collide with the arm64 set at `:latest`), and `compute.tf`'s
`templatefile()` call passes a different `image_tag` into the (still
byte-for-byte-shared) `cloud-init.yaml.tftpl` as a result. Still no
Elastic Container Registry, no second script — the only new thing is a
second *tag*, not a second *registry* or *pipeline*. Also worth noting:
AWS has no OCI/Azure-style "pass an SSH key directly as instance
metadata" mechanism (needs an explicit `aws_key_pair` resource), and a
custom `aws_security_group` needs an explicit egress rule for outbound
access the way OCI's NSGs do (Azure's NSGs are the one of the three with
a built-in default) — two more small but real per-provider differences,
confirmed against the actual `hashicorp/aws` provider before writing any
`.tf`, not assumed from the OCI/Azure pattern.
