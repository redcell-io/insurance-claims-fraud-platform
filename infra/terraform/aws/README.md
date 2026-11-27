# infra/terraform/aws

Provisions a single EC2 `t3.medium` (x86_64, 4GB) and runs the full
claim-fraud-platform deploy stack on it — a third path alongside
`infra/terraform/oci/` (blocked on Ampere A1 capacity) and
`infra/terraform/azure/` (built, access currently unavailable). See
DECISIONS.md's entry on this for the full rationale.

**This is a real, paid instance — not free-tier.** `t2.micro`/
`t3.micro` (the free-tier-eligible sizes) only have 1GB RAM, the same
resource-starvation risk already rejected for GCP's e2-micro at the
very start of this effort (the full stack measures ~800MB+ RSS at idle
alone). `t3.medium` is a deliberate tradeoff: a small, real cost in
exchange for actually fitting. **Verify it works, then tear it down
immediately** with `scripts/teardown-cloud.sh aws` — that's the real
cost-control mechanism here, not an afterthought.

Scope is deliberately narrow, same as the other two clouds: this
provisions **networking + the VM only**. The application layer is
`docker-compose.deploy.yml` (DECISIONS.md #19), shipped to the instance
via cloud-init rather than a git clone.

**Reuses OCIR, not an AWS-native registry** — same decision as Azure,
but this VM is x86_64, not arm64, so it pulls a *separate* image set
(tag `latest-amd64`, built via `PLATFORM=linux/amd64 IMAGE_TAG=latest-amd64
scripts/build-and-push-oci.sh`) rather than reusing the arm64 set
untouched. Still the same single script, same registry — no Elastic
Container Registry, no second build pipeline.

## Prerequisites

1. **A dedicated, scoped AWS CLI profile** — this deployment creates and
   destroys real paid resources, so it runs as its own narrowly-scoped
   IAM user (`AmazonEC2FullAccess` only) rather than any broader
   personal/admin credentials:
   ```bash
   aws iam create-user --user-name claimsfraud-terraform
   aws iam attach-user-policy --user-name claimsfraud-terraform \
     --policy-arn arn:aws:iam::aws:policy/AmazonEC2FullAccess
   aws iam create-access-key --user-name claimsfraud-terraform
   # write the resulting key into a named profile, NOT the default:
   aws configure set aws_access_key_id <key> --profile claimfraud
   aws configure set aws_secret_access_key <secret> --profile claimfraud
   aws configure set region us-east-1 --profile claimfraud
   ```
   Verify with `aws sts get-caller-identity --profile claimfraud` —
   confirm the ARN shows `claimsfraud-terraform`, not whichever user
   created it.
2. **The same SSH keypair and OCIR credentials already set up for
   `infra/terraform/oci`/`azure`** — all live in `../shared.tfvars`, not
   duplicated here.
3. **The x86_64 image set already pushed to OCIR** (see "Usage" below)
   — AWS pulls `:latest-amd64`, a different tag than the other two
   clouds' `:latest` (arm64).
4. **`terraform`** (already on `PATH` in this repo).

## Usage

```bash
cd infra/terraform/aws
cp ../shared.tfvars.example ../shared.tfvars   # if you haven't already — shared across clouds
cp terraform.tfvars.example terraform.tfvars   # AWS-specific values; gitignored
terraform init
terraform plan -var-file=../shared.tfvars -var-file=terraform.tfvars    # review before applying — this creates real, BILLABLE resources
terraform apply -var-file=../shared.tfvars -var-file=terraform.tfvars
```

If the x86_64 image set isn't on OCIR yet, build and push it first, from
the repo root:
```bash
proto/generate.sh   # proto/gen/go must exist before building — see DECISIONS.md #19
PLATFORM=linux/amd64 IMAGE_TAG=latest-amd64 \
  OCIR_REGION=... OCIR_NAMESPACE=... \
  ./scripts/build-and-push-oci.sh
```

`terraform output` prints the instance's public IP, a ready-to-paste
`ssh_command`, and reminds you of the REST/Prometheus URLs. Give
cloud-init a minute or two after `apply` finishes — `ssh` in and `sudo
tail -f /var/log/cloud-init-output.log` to watch it install Docker, log
into OCIR, and bring the stack up.

**Verify** with TC-01 from `docs/manual-test-cases.md`, against the
instance's public IP instead of `localhost`:

```bash
curl -s -X POST http://<instance_public_ip>:8080/v1/claims \
  -H "Content-Type: application/json" \
  -d '{"claim_id":"clm-aws-1","product":"auto","event_type":"claim.fnol","policy_number":"POL-123456","claimant_name":"Jane Doe","raw_address":"123 Main St, Springfield, IL 62704"}'
```

## Tearing down — do this promptly, this is a paid instance

```bash
scripts/teardown-cloud.sh aws
```
(or directly: `terraform destroy -var-file=../shared.tfvars
-var-file=terraform.tfvars`, from this directory). Either way, confirm
afterward with `terraform state list` — should be empty.

## What's deliberately not here (demo-grade, same framing as the other two clouds)

- **No TLS, no domain** — the REST API and Prometheus UI are plain HTTP
  on a bare public IP.
- **No remote Terraform state** — `terraform.tfstate` is local and
  gitignored. Fine solo; would need an S3 backend if more than one
  machine ever applies this.
- **No Elastic IP** — `associate_public_ip_address = true` on the
  instance is sufficient and avoids the idle-EIP-cost risk a forgotten
  Elastic IP would introduce for something meant to be short-lived.
- **No managed-identity/instance-profile auth for OCIR** — the auth
  token sits in plaintext in the rendered `user_data` (readable from the
  instance's own metadata service), same accepted tradeoff as the other
  two clouds. Not something an IAM instance profile would help with
  here anyway, since OCIR isn't an AWS-native registry.
- **No healthchecks on `depends_on`** — same race the other two clouds'
  READMEs document: a claim submitted seconds after boot can hit
  `DeadlineExceeded` while the Java services are still starting. Give it
  a minute.
- **No CI** — `scripts/build-and-push-oci.sh` is run by hand.
