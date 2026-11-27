#!/usr/bin/env bash
# Builds all 7 claim-fraud-platform service images and pushes them to
# OCIR. Run this from the repo root, after `proto/generate.sh`
# (proto/gen/go must exist on disk — see each service's Dockerfile and
# DECISIONS.md #19's "known gap").
#
# This is what actually resolves that gap: images are built here, where
# proto/gen/go already exists, and the VM only ever pulls them
# (docker-compose.deploy.images.yml) — it never runs `docker compose
# build` against a bare git clone. See DECISIONS.md #21.
#
# PLATFORM defaults to linux/arm64 — the OCI Ampere A1 Always Free shape
# (infra/terraform/oci) and Azure's B2pts v2 (infra/terraform/azure)
# both run it, NOT this machine's native arch, which is why `docker
# buildx build --platform ...` is required here instead of a plain
# `docker build`. infra/terraform/aws's t3.medium is x86_64 — build for
# it with `PLATFORM=linux/amd64 IMAGE_TAG=latest-amd64` (a distinct tag
# so it doesn't overwrite the arm64 set at `:latest`); still the same
# registry, no separate build pipeline per architecture.
#
# Requires: docker buildx (ships with modern Docker Desktop), and
# `docker login <region>.ocir.io` already done (or export
# OCIR_AUTH_TOKEN/OCIR_USERNAME below and this script will log in for
# you).
#
# Usage:
#   OCIR_REGION=iad OCIR_NAMESPACE=yourtenancynamespace ./scripts/build-and-push-oci.sh
#   PLATFORM=linux/amd64 IMAGE_TAG=latest-amd64 OCIR_REGION=iad OCIR_NAMESPACE=yourtenancynamespace ./scripts/build-and-push-oci.sh
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

: "${OCIR_REGION:?set OCIR_REGION, e.g. iad (us-ashburn-1) — see provider docs for the key matching your region}"
: "${OCIR_NAMESPACE:?set OCIR_NAMESPACE — find via: oci os ns get}"
OCIR_HOST="${OCIR_REGION}.ocir.io"
IMAGE_TAG="${IMAGE_TAG:-latest}"
PLATFORM="${PLATFORM:-linux/arm64}"

if [ -n "${OCIR_AUTH_TOKEN:-}" ] && [ -n "${OCIR_USERNAME:-}" ]; then
  echo "$OCIR_AUTH_TOKEN" | docker login "$OCIR_HOST" -u "$OCIR_USERNAME" --password-stdin
fi

if [ ! -d proto/gen/go ] || [ -z "$(ls -A proto/gen/go 2>/dev/null)" ]; then
  echo "proto/gen/go is missing/empty — run proto/generate.sh first (see DECISIONS.md #19)." >&2
  exit 1
fi

docker buildx inspect claimfraud-builder >/dev/null 2>&1 \
  || docker buildx create --name claimfraud-builder --use >/dev/null
docker buildx use claimfraud-builder

# service -> Dockerfile, same list/paths as docker-compose.deploy.yml.
declare -A services=(
  [tenant-config-svc]=services/tenant-config-svc/Dockerfile
  [model-service]=services/model-service/Dockerfile
  [claimant-id-hashing-svc]=services/claimant-id-hashing-svc/Dockerfile
  [address-normalization-svc]=services/address-normalization-svc/Dockerfile
  [policy-lookup-svc]=services/policy-lookup-svc/Dockerfile
  [orchestration-service]=services/orchestration-service/Dockerfile
  [claims-gateway]=services/claims-gateway/Dockerfile
)

for svc in "${!services[@]}"; do
  dockerfile="${services[$svc]}"
  tag="${OCIR_HOST}/${OCIR_NAMESPACE}/claimfraud/${svc}:${IMAGE_TAG}"
  echo "=== building + pushing ${tag} (${PLATFORM}) ==="
  docker buildx build \
    --platform "$PLATFORM" \
    -f "$dockerfile" \
    -t "$tag" \
    --push \
    .
done

echo "All 7 images pushed under ${OCIR_HOST}/${OCIR_NAMESPACE}/claimfraud/*:${IMAGE_TAG}"
