#!/usr/bin/env bash
# Retries `terraform apply` against infra/terraform/oci until the
# oci_core_instance launches, rotating Availability Domains each attempt.
#
# Ampere A1.Flex is Oracle's single most contended Always Free shape —
# "Out of host capacity" on a fresh launch is common and has nothing to
# do with the Terraform config (see DECISIONS.md's entry on this).
# Capacity is tracked per-AD, not per-region, and fluctuates as other
# tenants' instances churn, so the only real mitigation is: keep trying,
# rotating ADs, until one succeeds. This can take anywhere from minutes
# to days — there's no guaranteed timeframe.
#
# Stops immediately on success, or on any failure that ISN'T the known
# capacity error (auth failure, quota exceeded, a real config problem)
# — those need a human to look, not another blind retry.
#
# Usage: ./scripts/retry-oci-apply.sh [interval_seconds] [max_attempts]
set -uo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../infra/terraform/oci"

INTERVAL="${1:-180}"
MAX_ATTEMPTS="${2:-0}" # 0 = unlimited
AD_COUNT=3
attempt=0

echo "Starting retry loop: interval=${INTERVAL}s, max_attempts=${MAX_ATTEMPTS:-unlimited}"

while true; do
  attempt=$((attempt + 1))
  ad_index=$(((attempt - 1) % AD_COUNT))
  ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  echo "[$ts] attempt $attempt, availability_domain_index=$ad_index"

  output=$(terraform apply -auto-approve -input=false -var="availability_domain_index=$ad_index" 2>&1)
  rc=$?

  if [ "$rc" -eq 0 ]; then
    echo "[$ts] SUCCESS on attempt $attempt (AD index $ad_index)"
    echo "$output" | tail -20
    exit 0
  fi

  if echo "$output" | grep -qi "Out of host capacity"; then
    echo "[$ts] attempt $attempt failed: Out of host capacity (AD index $ad_index) — retrying"
  else
    echo "[$ts] attempt $attempt failed with a DIFFERENT error — stopping, this needs a human:"
    echo "$output" | tail -40
    exit 1
  fi

  if [ "$MAX_ATTEMPTS" -gt 0 ] && [ "$attempt" -ge "$MAX_ATTEMPTS" ]; then
    echo "[$ts] reached max_attempts=$MAX_ATTEMPTS without success, stopping"
    exit 2
  fi

  sleep "$INTERVAL"
done
