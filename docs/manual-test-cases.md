# Manual test cases — console-driven REST/gRPC verification

Reusable manual test cases for exercising claim-fraud-platform's REST and
gRPC surface through the
[all-in-one-end-to-end-test-console](/c/Users/mtand/Projects/Dev/MultiLanguage/all-in-one-end-to-end-test-console)
(a separate local project — Postman-like HTTP/gRPC testing with dynamic
proto parsing, no generated stubs needed). Re-run this whole file after any
change to an enrichment service, the DAG config, or Orchestration's
dispatch logic, instead of re-deriving the same curl/grpcurl commands from
scratch each time.

For *why* the platform is built this way, see [DESIGN.md](../DESIGN.md)
and [DECISIONS.md](../DECISIONS.md). For the curl-only smoke test and full
run order, see [RUNBOOK.md](../RUNBOOK.md). This doc only covers the
console-driven path.

## Prerequisites

1. All 7 claim-fraud-platform processes running — [RUNBOOK.md](../RUNBOOK.md)
   step 4.
2. The console, backend on a **non-default port** (its default `:8080`
   collides with ClaimsGateway):
   ```sh
   cd backend && PORT=8090 go run ./cmd/server               # terminal A
   cd frontend && BACKEND_PORT=8090 npm run dev               # terminal B
   ```
   `BACKEND_PORT` matters for the frontend too, not just the backend —
   its dev-proxy hardcodes `8080` otherwise and the browser UI silently
   fails (see the 2026-09-11 run note near the bottom of this file).
3. `jq` on `PATH` if driving the raw APIs from a shell instead of the UI
   (every request/response below is written as a raw `curl` call so it can
   be pasted directly — the console's frontend panels do the same thing
   with less typing, once its proto is uploaded).

## How to read each test case

Every test case states the same five fields, always in this order, so
none of them (port included) end up buried in prose:

- **Endpoint** — the exact service/method (gRPC) or HTTP method+path
  (REST) being exercised.
- **Port** — the exact `host:port` to put in the console's Target field
  (gRPC) or the target URL's authority (REST). Never says "same as
  TC-NN" — always the literal value, even when it repeats a prior test
  case's.
- **Proto** — which `.proto` file to upload/parse in the console's gRPC
  panel (gRPC only; omitted for REST test cases).
- **Payload** — exact JSON body to send.
- **Expected response** — what a healthy run returns.
- **Last verified** — most recent run's actual result and date, so this
  file doubles as a running log, not just a spec.

---

## TC-01 — ClaimsGateway REST, full chain (happy path)

Proves the whole request chain: ClaimsGateway → Orchestration → (claimant
ID hash + address normalization + policy lookup, in parallel) → Model
Service, all through the console's REST proxy rather than curling
ClaimsGateway directly.

**Endpoint**: `POST /api/v1/proxy` (console's own proxy endpoint) →
forwards to `POST /v1/claims` (ClaimsGateway)

**Port**: console backend `localhost:8090` → target `localhost:8080`

**Payload**:
```json
{
  "method": "POST",
  "url": "http://localhost:8080/v1/claims",
  "headers": {"Content-Type": ["application/json"]},
  "body": "{\"claim_id\":\"clm-console-001\",\"product\":\"auto\",\"event_type\":\"claim.fnol\",\"policy_number\":\"POL-123456\",\"claimant_name\":\"Jane Doe\",\"raw_address\":\"123 Main St, Springfield, IL 62704\"}"
}
```
Note: `headers` is `map[string][]string` — a plain string value
(`{"Content-Type": "application/json"}`) 400s with a JSON unmarshal error.

**Expected response**: `status: 200`, decoded `body.status: "scored"`,
`body.fraud_score: 0.42`.

**Last verified**: 2026-09-11 — `200`, `status:"scored"`,
`fraud_score:0.42`, `normalized_address:"123 MAIN ST, SPRINGFIELD, IL,
62704, US"`, via the actual browser UI (not just curl) for the first
time — see the 2026-09-11 run note below for why that distinction
mattered. ✅ (Previously verified 2026-09-10: same result shape,
`durationMs:524`.)

---

## TC-02 — tenant-config-svc: `GetTenant`

**Endpoint**: gRPC `tenantconfig.v1.TenantConfigService/GetTenant`

**Port**: `localhost:9094`

**Proto**: `proto/tenantconfig/v1/tenant_config.proto` (single uploaded
file)

**Payload**:
```json
{"tenant_id": "acme_insurance"}
```

**Expected response**: `{tenantId:"acme_insurance", displayName:"Acme
Insurance Co", status:"active"}`.

**Last verified**: 2026-09-11 — matched exactly. ✅ Also ad-hoc
negative-tested with a typo'd `tenant_id` (`"acme_insuranc"`) — got a
clean `rpc error: code = NotFound desc = tenant "acme_insuranc" not
found` rather than a silent/wrong success. Worth promoting to a formal
TC-02b if this file gets another pass.

---

## TC-03 — tenant-config-svc: `GetDagVersion`

**Endpoint**: gRPC `tenantconfig.v1.TenantConfigService/GetDagVersion`

**Port**: `localhost:9094`

**Proto**: `proto/tenantconfig/v1/tenant_config.proto` (same upload as
TC-02)

**Payload**:
```json
{"tenant_id": "acme_insurance", "product": "auto"}
```

**Expected response**: `{"version": 2}` — matches
`config/tenants.yaml`'s `dag_versions.auto`.

**Last verified**: 2026-09-11 — `{"version":2}`. ✅

---

## TC-04 — address-normalization-svc: `Normalize`

**Endpoint**: gRPC `addressnorm.v1.AddressNormalizationService/Normalize`

**Port**: `localhost:9092`

**Proto**: `proto/addressnorm/v1/address_normalization.proto`

**Payload**:
```json
{"correlation_id": "console-test-1", "raw_address": "123 Main St, Springfield, IL 62704"}
```

**Expected response**: normalized address breakdown with `valid:true`.

**Last verified**: 2026-09-11 —
`{"normalizedAddress":"123 MAIN ST, SPRINGFIELD, IL, 62704, US",
"line1":"123 MAIN ST","city":"SPRINGFIELD","state":"IL",
"postalCode":"62704","country":"US","valid":true}`. ✅

---

## TC-05 — claimant-id-hashing-svc: `HashClaimantId`

**Endpoint**: gRPC
`claimantidhash.v1.ClaimantIdHashingService/HashClaimantId`

**Port**: `localhost:9095`

**Proto**: `proto/claimantidhash/v1/claimant_id_hashing.proto`

**Payload**:
```json
{"correlation_id": "console-test-1", "claimant_name": "Jane Doe"}
```

**Expected response**: a 64-hex-char SHA-256 hash in `claimantIdHash`,
stable across repeat calls with the same `claimant_name`
(case/whitespace normalized — see [DECISIONS.md](../DECISIONS.md) #12).

**Last verified**: 2026-09-11 —
`{"claimantIdHash":"7b9d77204ff21b167f8936bf0deefef40eee95574b6c9432fa890e5b6135065f"}`
— byte-for-byte identical to the 2026-09-10 run, confirming the hash is
deterministic. ✅

---

## TC-06 — policy-lookup-svc: `LookupPolicy`, known policy

**Endpoint**: gRPC `policylookup.v1.PolicyLookupService/LookupPolicy`

**Port**: `localhost:9096`

**Proto**: `proto/policylookup/v1/policy_lookup.proto`

**Payload**:
```json
{"correlation_id": "console-test-1", "policy_number": "POL-123456"}
```

**Expected response**: `{"found":true, "status":"active",
"coverageType":"full"}`.

**Last verified**: 2026-09-10 — matched exactly. ✅ (Not re-run
2026-09-11 — due next pass.)

---

## TC-07 — policy-lookup-svc: `LookupPolicy`, unknown policy

Confirms an unrecognized policy number is a normal *response*, not an
RPC error — see [DECISIONS.md](../DECISIONS.md) #13.

**Endpoint**: gRPC `policylookup.v1.PolicyLookupService/LookupPolicy`
(same as TC-06)

**Port**: `localhost:9096`

**Proto**: `proto/policylookup/v1/policy_lookup.proto` (same as TC-06)

**Payload**:
```json
{"correlation_id": "console-test-2", "policy_number": "POL-BOGUS-999"}
```

**Expected response**: `found` absent/false, `status:"unknown"`, no RPC
error.

**Last verified**: 2026-09-11 — `{"status":"unknown"}`, no error. ✅
(Note: shape differs slightly from the 2026-09-10 run —
`{"response":{"status":"unknown"},"durationMs":1}` — no `response`
wrapper or `durationMs` this time. Not a regression: `found`
absent/false and `status:"unknown"` both hold either way, which is all
this test case actually checks; looks like console UI just renders the
decoded message differently run to run.)

---

## TC-08 — model-service: `Score`

**Endpoint**: gRPC `model.v1.ModelService/Score`

**Port**: `localhost:9093`

**Proto**: `proto/model/v1/model.proto`

**Payload**:
```json
{"correlation_id": "console-test-1", "features": {"tenant_id": "acme_insurance"}}
```

**Expected response**: `{"score":0.42, "modelVersion":"stub-v0"}`
(hardcoded stub — see [DESIGN.md](../DESIGN.md)/[DECISIONS.md](../DECISIONS.md)
for the real ONNX/Triton model still pending).

**Last verified**: 2026-09-10 — matched exactly. ✅ (Not re-run
2026-09-11 — due next pass.)

---

## TC-09 — OrchestrationService: `ProcessClaim` via direct gRPC (known limitation)

Not a pass/fail test of claim-fraud-platform — this documents a **console
limitation** so it isn't re-discovered from scratch. `orchestration.proto`
imports `claims/v1/claims.proto`, which the console's frontend can't parse
(single-file upload only); the raw `/api/v1/grpc/parse` API supports
multi-file via its `files` map, so the import itself is *not* the blocker
it first looks like. The real blocker: `claims.v1.ClaimEvent` deliberately
excludes `tenant_id` from the payload — it travels as `x-tenant-id` gRPC
metadata, injected by ClaimsGateway's interceptor (see
[DESIGN.md](../DESIGN.md) §8). The console's `InvokeRequest` has no
metadata field, so a direct call always resolves tenant to `""`.

**Endpoint**: gRPC `orchestration.v1.OrchestrationService/ProcessClaim`

**Port**: `localhost:9091`

**Proto**: `orchestration/v1/orchestration.proto` +
`claims/v1/claims.proto` (multi-file upload via the raw
`/api/v1/grpc/parse` API's `files` map — not available in the frontend
panel today)

**Workaround (until the console's frontend supports multi-file
upload — tracked in its `docs/ROADMAP.md`)**: run these two curls from
the repo root (Git Bash; `jq` required for the first one) instead of
using the GRPC panel's Parse/Invoke buttons.

Parse (multi-file, via the backend's raw API — build the `files` map
from the actual proto sources so it never drifts from what's on disk):
```sh
jq -n \
  --arg entry "orchestration/v1/orchestration.proto" \
  --rawfile orch "proto/orchestration/v1/orchestration.proto" \
  --rawfile claims "proto/claims/v1/claims.proto" \
  '{entryFile: $entry, files: {"orchestration/v1/orchestration.proto": $orch, "claims/v1/claims.proto": $claims}}' \
  > /tmp/parse_request.json

curl -s -X POST http://localhost:8090/api/v1/grpc/parse \
  -H "Content-Type: application/json" \
  -d @/tmp/parse_request.json
```
Copy the `hash` from the response into the invoke call below.

Invoke:
```sh
curl -s -X POST http://localhost:8090/api/v1/grpc/invoke \
  -H "Content-Type: application/json" \
  -d '{
    "hash": "<hash from the Parse response above>",
    "service": "orchestration.v1.OrchestrationService",
    "method": "ProcessClaim",
    "target": "localhost:9091",
    "request": {"claim": {"claim_id": "clm-console-002", "product": "auto", "event_type": "claim.fnol", "policy_number": "POL-123456", "claimant_name": "Jane Doe", "raw_address": "123 Main St, Springfield, IL 62704"}}
  }'
```

**Payload** (the `request` field of the Invoke call above; shown
standalone too, matching every other test case's format in this
file):
```json
{"claim": {"claim_id": "clm-console-002", "product": "auto", "event_type": "claim.fnol", "policy_number": "POL-123456", "claimant_name": "Jane Doe", "raw_address": "123 Main St, Springfield, IL 62704"}}
```

**Expected response** (given the console's current limitation):
`Parse` succeeds; `Invoke` returns a clean gRPC `NotFound` — *not* a
hang, timeout, or silent empty response — because tenant resolves to
`""`.

**Last verified**: 2026-09-11 — Parse succeeded via the raw
`/api/v1/grpc/parse` API with a `files` map (frontend single-file
upload still fails identically to 2026-09-10, confirmed again this
session — see below). Invoke →
`{"error":"invoking ProcessClaim: rpc error: code = NotFound desc = load
dag config: resolve dag version: rpc error: code = NotFound desc = no dag
version configured for tenant \"\" product \"auto\""}`. Confirms the
failure mode is clean, not broken — this is expected until the console
gains metadata support (tracked in that repo's own `skills/grpc.md`).
**Use TC-01 (REST) to actually exercise OrchestrationService's logic** —
this test case exists only to confirm the gRPC path's known ceiling, not
as a substitute for TC-01. (Previously verified 2026-09-10: identical
result.)

**Console-side multi-file-upload gap now tracked as a feature
request** in that repo's `docs/ROADMAP.md` ("Known open gaps") — see
`skills/grpc.md`'s 2026-09-11 entry for the fix shape.

---

## 2026-09-10 full run summary

All of TC-01 through TC-08 passed on the first run, back-to-back, with all
7 claim-fraud-platform processes + the console (backend `:8090`, frontend
`:5173`) up simultaneously — no restarts needed between test cases. TC-09
behaved exactly as its "known limitation" expects. Two console-side
findings from this run were fed back into that repo's own docs
(`skills/grpc.md`, `docs/MANUAL_TESTING.md`): the `headers` shape gotcha
(TC-01) and the missing-metadata gap (TC-09).

This run did **not** repeat the `on_failure` fail-fast/degrade resilience
checks (stopping a service mid-run) — those were already verified via
plain curl in the prior session; see [DECISIONS.md](../DECISIONS.md) #12
and #14 for that result. Worth re-running through the console next time
(kill `claimant-id-hashing-svc`, re-invoke TC-05 or re-run TC-01, confirm
the panel/response surfaces a real connection error rather than hanging)
if this file is picked up again.

---

## 2026-09-11 continued run

Session resumed after an accidental close mid-way through the
2026-09-10 run above. Before re-testing, found and fixed a real bug in
the console project itself: `frontend/vite.config.ts` hardcoded its
dev-proxy target to `127.0.0.1:8080`, but the backend must run on
`8090` per this doc's own prerequisites — so the *browser* path (as
opposed to curl) silently 500'd with an empty body, looking exactly
like a backend failure. Fixed via a `BACKEND_PORT` env var; see that
repo's `skills/httpproxy.md`. Launch its frontend with
`BACKEND_PORT=8090 npm run dev` going forward (folded into this file's
own Prerequisites section above).

TC-01 through TC-05, TC-07, and TC-09 re-verified this session (see
each test case's updated "Last verified" line above) — all passed, all
matching the prior run's results; TC-01 through TC-05 and TC-07 through
the browser UI, TC-09 via the raw API (as its known limitation
requires). TC-06 and TC-08 were **not** re-run this session; still only
verified as of 2026-09-10 (flagged inline on each). Also ran an ad-hoc
negative test alongside TC-02 (see its note above) worth formalizing as
TC-02b later, and promoted TC-09's console limitation from a skill-file
note to a tracked feature request in the console repo's own
`docs/ROADMAP.md` (see TC-09's entry above).

Also reformatted every test case in this pass to a strict five-field
order (**Endpoint** / **Port** / **Proto** / **Payload** / **Expected
response** / **Last verified**), with **Port** always stated as a
literal `host:port` — never "same as TC-NN" — after that phrasing on
TC-07 cost a round of confusion about which port to actually use.
