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

```sh
cd infra/terraform/oci
cp terraform.tfvars.example terraform.tfvars   # fill in real values; gitignored
terraform init
terraform plan    # review before applying — this creates real, billable-adjacent resources
terraform apply
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
terraform destroy
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

## Porting to AWS later

Don't expect this directory's `.tf` files to port — OCI's resource
model (compartments, VCNs, NSGs) and AWS's (VPC, EC2 security groups)
are different enough that an `infra/terraform/aws/` later is a rewrite
of the resource blocks, not a find-and-replace. What *does* carry over:
the same `docker-compose.deploy.images.yml` + registry-pull pattern
(swap OCIR for ECR), the same cloud-init `write_files` approach, and the
same `terraform plan`/`apply`/`destroy` workflow and variable-naming
conventions.
