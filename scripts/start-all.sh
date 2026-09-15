#!/usr/bin/env bash
# Starts all seven claim-fraud-platform services in the order documented
# in RUNBOOK.md step 4b (tenant-config-svc first, everyone depends on it;
# ClaimsGateway last). Each is backgrounded with its own log file under
# ./logs/.
#
# If any of the seven ports is already bound (e.g. leftover processes from
# an earlier session — see the go-toolchain/port-collision notes in
# RUNBOOK.md), this warns and asks for confirmation before killing the
# owning process(es) and proceeding. Pass -y/--yes to skip the prompt
# (e.g. for scripting).
#
# Does NOT start the Kafka broker (`docker compose up -d`, RUNBOOK.md step
# 4a) or the test console — this only covers the seven platform services,
# matching exactly what RUNBOOK.md's step 4b documents.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

AUTO_YES=false
if [[ "${1:-}" == "-y" || "${1:-}" == "--yes" ]]; then
  AUTO_YES=true
fi

# Order matters — must match RUNBOOK.md step 4b exactly.
ORDER=(tenant-config-svc model-service claimant-id-hashing-svc address-normalization-svc policy-lookup-svc orchestration-service claims-gateway)

declare -A PORT=(
  [tenant-config-svc]=9094
  [model-service]=9093
  [claimant-id-hashing-svc]=9095
  [address-normalization-svc]=9092
  [policy-lookup-svc]=9096
  [orchestration-service]=9091
  [claims-gateway]=8080
)

declare -A DIR=(
  [tenant-config-svc]=services/tenant-config-svc
  [model-service]=services/model-service
  [claimant-id-hashing-svc]=services/claimant-id-hashing-svc
  [address-normalization-svc]=services/address-normalization-svc
  [policy-lookup-svc]=services/policy-lookup-svc
  [orchestration-service]=services/orchestration-service
  [claims-gateway]=services/claims-gateway
)

declare -A CMD=(
  [tenant-config-svc]="go run ./cmd/tenantconfig"
  [model-service]="go run ./cmd/model"
  [claimant-id-hashing-svc]="mvn spring-boot:run"
  [address-normalization-svc]="mvn spring-boot:run"
  [policy-lookup-svc]="mvn spring-boot:run"
  [orchestration-service]="go run ./cmd/orchestration"
  [claims-gateway]="go run ./cmd/gateway"
)

PORT_LIST=$(IFS=,; echo "${PORT[*]}")

echo "Checking ports: $PORT_LIST ..."

# One combined PowerShell call (not one per port) — cheaper, and mirrors
# the check used interactively earlier in this project's sessions.
#
# Written to a temp .ps1 file and run with -File, not passed inline via
# -Command "..." — the inline form silently breaks here: Write-Output's own
# embedded double quotes collide with the outer -Command "..." boundary once
# Git Bash's argument expansion flattens everything into a single Windows
# command-line string, and PowerShell's resulting parse error was getting
# swallowed by `2>/dev/null`, so the script just silently died after
# printing "Checking ports:" with no explanation. A file has no such
# quoting boundary to collide with.
PS_CHECK_SCRIPT="/tmp/claim-fraud-start-all-check-$$.ps1"
cat > "$PS_CHECK_SCRIPT" <<EOF
\$ports = '$PORT_LIST' -split ','
foreach (\$p in \$ports) {
  \$c = Get-NetTCPConnection -LocalPort \$p -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
  if (\$c) {
    \$proc = Get-Process -Id \$c.OwningProcess -ErrorAction SilentlyContinue
    Write-Output "\$p|\$(\$c.OwningProcess)|\$(\$proc.ProcessName)"
  }
}
EOF
CONFLICTS_RAW=$(powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$PS_CHECK_SCRIPT" 2>/dev/null | tr -d '\r')
rm -f "$PS_CHECK_SCRIPT"

declare -A PORT_TO_NAME
for name in "${ORDER[@]}"; do
  PORT_TO_NAME["${PORT[$name]}"]="$name"
done

CONFLICT_PIDS=()
if [[ -n "$CONFLICTS_RAW" ]]; then
  echo
  echo "The following ports are already in use:"
  while IFS='|' read -r port pid pname; do
    [[ -z "$port" ]] && continue
    svc="${PORT_TO_NAME[$port]:-unknown}"
    echo "  :$port ($svc) — PID $pid ($pname)"
    CONFLICT_PIDS+=("$pid")
  done <<< "$CONFLICTS_RAW"
fi

if [[ ${#CONFLICT_PIDS[@]} -gt 0 ]]; then
  echo
  echo "This will kill ${#CONFLICT_PIDS[@]} process(es) listed above before starting."
  if [[ "$AUTO_YES" != true ]]; then
    read -r -p "Proceed? [y/N] " REPLY
    if [[ ! "$REPLY" =~ ^[Yy]$ ]]; then
      echo "Aborted — nothing killed, nothing started."
      exit 1
    fi
  fi
  for pid in "${CONFLICT_PIDS[@]}"; do
    echo "Killing PID $pid..."
    powershell.exe -NoProfile -Command "Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue" >/dev/null
  done
  echo "Waiting for ports to free up..."
  sleep 2
fi

mkdir -p logs
echo
echo "Starting all seven services (logs in $REPO_ROOT/logs/*.log)..."
for name in "${ORDER[@]}"; do
  dir="$REPO_ROOT/${DIR[$name]}"
  cmd="${CMD[$name]}"
  port="${PORT[$name]}"
  logfile="$REPO_ROOT/logs/$name.log"
  echo "  [$name] :$port -> ${DIR[$name]} ($cmd)"
  ( cd "$dir" && nohup $cmd > "$logfile" 2>&1 & )
  sleep 1   # small stagger for readable logs; each service dials downstream lazily so exact timing isn't required
done

echo
echo "All seven services launched."
echo "  Tail logs:   tail -f logs/*.log"
echo "  Smoke test:  curl -s -X POST http://localhost:8080/v1/claims -H 'Content-Type: application/json' -d '{\"claim_id\":\"clm-script-1\",\"product\":\"auto\",\"event_type\":\"claim.fnol\",\"policy_number\":\"POL-123456\",\"claimant_name\":\"Jane Doe\",\"raw_address\":\"123 Main St, Springfield, IL 62704\"}'"
echo "  Kafka broker (docker compose up -d) and the test console are NOT started by this script — see RUNBOOK.md steps 4a and the console's own docs."
