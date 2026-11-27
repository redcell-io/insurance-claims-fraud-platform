#!/usr/bin/env bash
# Thin wrapper around `terraform destroy` for any of the three cloud
# deploy targets (infra/terraform/<oci|azure|aws>/) — exists purely so
# the two -var-file flags don't have to be retyped/remembered each time,
# especially for paid resources (AWS's t3.medium) meant to be torn down
# right after verification, not left running.
#
# Deliberately does NOT pass -auto-approve: destroy is the one
# operation in this whole effort where skipping the interactive "type
# yes" confirmation would be a real safety regression, not just
# friction. This stays a safe wrapper, not a one-command skip-the-
# confirmation tool.
#
# Usage: ./scripts/teardown-cloud.sh <oci|azure|aws>
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

cloud="${1:-}"
case "$cloud" in
  oci | azure | aws) ;;
  *)
    echo "Usage: $0 <oci|azure|aws>" >&2
    exit 1
    ;;
esac

dir="infra/terraform/${cloud}"
if [ ! -d "$dir" ]; then
  echo "No such directory: $dir" >&2
  exit 1
fi

cd "$dir"
terraform destroy -var-file=../shared.tfvars -var-file=terraform.tfvars
