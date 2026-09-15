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
   step 4b — **plus the Kafka broker from step 4a** (`docker compose up
   -d`) if you're running TC-10. Kafka connection details, for the
   console's Kafka panel (Brokers / Topics fields):
   | Field | Value |
   |---|---|
   | Brokers | `localhost:19092` |
   | Topics | `claims.realtime` |
   | Decode values as Avro | unchecked — plain JSON, no schema registry (see [DECISIONS.md](../DECISIONS.md) #15) |

   Connect *before* submitting a claim — the panel's consumer starts from
   the topic's latest offset, so a message produced before you click
   Connect won't show up. Full walkthrough: TC-10 below.
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

Every test case states the same six fields, always in this order, so
none of them (protocol and port included) end up buried in prose or only
implied by which console panel a screenshot would use:

- **Protocol** — `REST`, `gRPC`, or `Kafka`. Stated as its own field, not
  left to be inferred from the Endpoint line — TC-01 used to be ambiguous
  about this (REST, but routed through the console's gRPC-adjacent-looking
  proxy machinery) until this field was added.
- **Endpoint** — the exact service/method (gRPC) or HTTP method+path
  (REST) being exercised.
- **Port** — the exact `host:port` to put in the console's Target field
  (gRPC) or the target URL's authority (REST). Never says "same as
  TC-NN" — always the literal value, even when it repeats a prior test
  case's.
- **Proto file** — which `.proto` file to upload/parse in the console's
  gRPC panel (gRPC only; omitted for REST/Kafka test cases). Named "Proto
  file," not "Proto," specifically so it doesn't read like a second,
  competing **Protocol** field two lines above it.
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

**Protocol**: REST (plain HTTP — not gRPC. The console's HTTP Request
panel sends this by calling its own `POST /api/v1/proxy`, which then
forwards it as a normal HTTP request; that's an implementation detail of
the console, not a second protocol hop worth confusing with gRPC.)

**Endpoint**: `POST /v1/claims` (ClaimsGateway) — sent via the console's
HTTP proxy, `POST /api/v1/proxy`

**Port**: target `localhost:8080` (ClaimsGateway) — console backend
`localhost:8090` is just the relay, not what you're testing; it's never
typed into the panel, the UI already knows its own backend address

**Console UI steps** (Request panel — what to actually type in each
field; this is the normal way to run this test case):
1. **Method**: `POST`
2. **URL**: `http://localhost:8080/v1/claims` — the real target, always
   ClaimsGateway's actual address, never the console's own `:8090`
3. **Headers**: one row, key `Content-Type`, value `application/json`
4. **Body**:
   ```json
   {"claim_id":"clm-console-001","product":"auto","event_type":"claim.fnol","policy_number":"POL-123456","claimant_name":"Jane Doe","raw_address":"123 Main St, Springfield, IL 62704"}
   ```
5. Click **Send**.

**Payload** (equivalent raw JSON body, only relevant if driving
`/api/v1/proxy` directly via curl instead of the UI — see Prerequisites
#3; the UI steps above build this for you, you don't construct it by
hand):
```json
{
  "method": "POST",
  "url": "http://localhost:8080/v1/claims",
  "headers": {"Content-Type": ["application/json"]},
  "body": "{\"claim_id\":\"clm-console-001\",\"product\":\"auto\",\"event_type\":\"claim.fnol\",\"policy_number\":\"POL-123456\",\"claimant_name\":\"Jane Doe\",\"raw_address\":\"123 Main St, Springfield, IL 62704\"}"
}
```
Note: `headers` is `map[string][]string` — a plain string value
(`{"Content-Type": "application/json"}`) 400s with a JSON unmarshal error
(only matters for the raw-curl path; the UI's header rows handle this
correctly on their own).

**Expected response**: `status: 200`, decoded `body.status: "scored"`,
`body.fraud_score: 0.42`.

**Last verified**: 2026-09-13 — `200 OK`, `1045ms`, `status:"scored"`,
`fraud_score:0.42`, `normalized_address:"123 MAIN ST, SPRINGFIELD, IL,
62704, US"`, via the actual browser UI. ✅ (Previously verified
2026-09-11: same result shape, via UI for the first time that session —
see that run note below. 2026-09-10: `durationMs:524`, same shape.)

---

## TC-02 — tenant-config-svc: `GetTenant`

**Protocol**: gRPC

**Endpoint**: gRPC `tenantconfig.v1.TenantConfigService/GetTenant`

**Port**: `localhost:9094`

**Proto file**: `proto/tenantconfig/v1/tenant_config.proto` (single
uploaded file)

**Payload**:
```json
{"tenant_id": "acme_insurance"}
```

**Expected response**: `{tenantId:"acme_insurance", displayName:"Acme
Insurance Co", status:"active"}`.

**Last verified**: 2026-09-11 — matched exactly. ✅ Also ad-hoc
negative-tested with a typo'd `tenant_id` — see TC-02b below, formalized
from this note.

---

## TC-02b — tenant-config-svc: `GetTenant`, unknown tenant

Confirms an unrecognized `tenant_id` is a clean gRPC error, not a silent
wrong/empty success — mirrors TC-07's role for policy-lookup-svc, but for
a real NotFound instead of a normal found:false response (see
`internal/server/tenantconfig.go`'s `status.Errorf(codes.NotFound, ...)`).

**Protocol**: gRPC

**Endpoint**: gRPC `tenantconfig.v1.TenantConfigService/GetTenant` (same
as TC-02)

**Port**: `localhost:9094`

**Proto file**: `proto/tenantconfig/v1/tenant_config.proto` (same as
TC-02)

**Payload**:
```json
{"tenant_id": "acme_insuranc"}
```
(typo'd — one character short of the real `acme_insurance`)

**Expected response**: a clean gRPC error, not `200`/success with empty
fields — `rpc error: code = NotFound desc = tenant "acme_insuranc" not
found`.

**Worth knowing**: this request produces **no server-side log line at
all**, success or error — same cause as TC-03b's note (`tenant-config-svc`
has zero `log.Printf` calls in either RPC handler). The gRPC error is
visible in the console's Response panel regardless; `logs/tenant-config-svc.log`
stays empty either way.

**Last verified**: 2026-09-14 — re-run live via the console's gRPC panel
with a different unknown `tenant_id` (`"fake_insurance_insurance-tenant"`),
got the expected `NotFound` error, and confirmed live via a tailed
`tenant-config-svc.log` panel that nothing was written to the log ("No
lines yet"). ✅ (Originally ad-hoc tested 2026-09-11 alongside TC-02;
formalized as its own test case 2026-09-14.)

---

## TC-03 — tenant-config-svc: `GetDagVersion`

**Protocol**: gRPC

**Endpoint**: gRPC `tenantconfig.v1.TenantConfigService/GetDagVersion`

**Port**: `localhost:9094`

**Proto file**: `proto/tenantconfig/v1/tenant_config.proto` (same upload
as TC-02)

**Payload**:
```json
{"tenant_id": "acme_insurance", "product": "auto"}
```

**Expected response**: `{"version": 3}` — matches
`config/tenants.yaml`'s `dag_versions.auto`.

**Last verified**: 2026-09-14 — re-run live via the console's gRPC panel,
`{"version":3}`, matching the corrected expected value exactly. ✅
(Previously verified 2026-09-11 at `{"version":2}`, before
`dag_versions.auto` was bumped 2→3 alongside the Kafka publish work — see
DECISIONS.md #15.)

---

## TC-03b — tenant-config-svc: `GetDagVersion`, missing `product`

Confirms an incomplete request (required-in-practice field omitted, not
just wrong) is a clean gRPC error, not a silent wrong/empty success or a
crash — same spirit as TC-02b, different flavor of bad input (missing
field vs. wrong value).

**Protocol**: gRPC

**Endpoint**: gRPC `tenantconfig.v1.TenantConfigService/GetDagVersion`
(same as TC-03)

**Port**: `localhost:9094`

**Proto file**: `proto/tenantconfig/v1/tenant_config.proto` (same as
TC-03)

**Payload**:
```json
{"tenant_id": "acme_insurance"}
```
(`product` omitted entirely — proto3 defaults it to `""` rather than
rejecting the request at the framing level)

**Expected response**: a clean gRPC error —
`rpc error: code = NotFound desc = no dag version configured for tenant
"acme_insurance" product ""` (empty `product` just fails the same
map-lookup miss as any other unconfigured value — see
`internal/server/tenantconfig.go`'s `GetDagVersion`).

**Worth knowing**: this request produces **no server-side log line at
all**, success or error — `tenant-config-svc` has zero `log.Printf` calls
in either RPC handler (unlike ClaimsGateway, which at least logs its
error branches). The gRPC error above is visible in the console's
Response panel regardless; it just won't show up in
`logs/tenant-config-svc.log`. See the 2026-09-14 note below for the
broader logging gap this surfaces.

**Last verified**: 2026-09-14 — re-run live via the console's gRPC panel,
got the exact predicted error verbatim: `rpc error: code = NotFound desc
= no dag version configured for tenant "acme_insurance" product ""`. ✅
(Previously only derived from source, not run live — see prior note.)

---

## TC-04 — address-normalization-svc: `Normalize`

**Protocol**: gRPC

**Endpoint**: gRPC `addressnorm.v1.AddressNormalizationService/Normalize`

**Port**: `localhost:9092`

**Proto file**: `proto/addressnorm/v1/address_normalization.proto`

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

**Protocol**: gRPC

**Endpoint**: gRPC
`claimantidhash.v1.ClaimantIdHashingService/HashClaimantId`

**Port**: `localhost:9095`

**Proto file**: `proto/claimantidhash/v1/claimant_id_hashing.proto`

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

**Protocol**: gRPC

**Endpoint**: gRPC `policylookup.v1.PolicyLookupService/LookupPolicy`

**Port**: `localhost:9096`

**Proto file**: `proto/policylookup/v1/policy_lookup.proto`

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

**Protocol**: gRPC

**Endpoint**: gRPC `policylookup.v1.PolicyLookupService/LookupPolicy`
(same as TC-06)

**Port**: `localhost:9096`

**Proto file**: `proto/policylookup/v1/policy_lookup.proto` (same as TC-06)

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

**Protocol**: gRPC

**Endpoint**: gRPC `model.v1.ModelService/Score`

**Port**: `localhost:9093`

**Proto file**: `proto/model/v1/model.proto`

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

**Protocol**: gRPC

**Endpoint**: gRPC `orchestration.v1.OrchestrationService/ProcessClaim`

**Port**: `localhost:9091`

**Proto file**: `orchestration/v1/orchestration.proto` +
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

## TC-10 — Kafka: scored claim published to `claims.realtime`

Confirms the `publish` DAG node's real Kafka publish (DECISIONS.md #15,
replacing the old log-only stub) actually lands a message on the broker,
using the console's Kafka streaming panel rather than curl/grpcurl — the
first test case in this file that isn't HTTP or gRPC.

**Protocol**: Kafka (not REST, not gRPC — the console's Kafka streaming
panel, a third panel type distinct from the other two)

**Endpoint**: Kafka topic `claims.realtime` (console's Kafka panel, not
`/api/v1/proxy` or `/api/v1/grpc/*`)

**Port**: `localhost:19092` (the broker from RUNBOOK.md step 4a — not
`9092`, which address-normalization-svc's gRPC server already owns in
this repo)

**Proto file**: none — messages are plain JSON (DECISIONS.md #15), so leave
"Decode values as Avro" **unchecked** in the console's Kafka panel.

**Payload**: none to send — this test case *observes* a message produced
by TC-01 (or any other happy-path claim submission), it doesn't submit
one itself. Steps:
1. In the console's Kafka panel: Brokers = `localhost:19092`, Topics =
   `claims.realtime`, click **Connect**.
2. Run TC-01 (or any `POST /v1/claims`) against a live stack.
3. Watch the message arrive in the panel.

**Expected response**: a JSON message appears, keyed by `tenant_id`
(`acme_insurance`), with `claim_id`/`correlation_id` matching the claim
just submitted, `status:"scored"`, `fraud_score:0.42`,
`model_version:"stub-v0"`.

**Last verified**: 2026-09-13 — **first time actually clicked through in
the console's own browser UI** (previously only verified via a throwaway
`kafka-go` consumer, see below). Connected the Kafka panel
(`localhost:19092`, `claims.realtime`, Avro off), ran TC-01, message
arrived in the panel within about a second:
```
claims.realtime · p0@1 · 8:28:23 AM
key: acme_insurance
{"claim_id":"clm-console-001","correlation_id":"3c39f8321fd24acd8d2b9f30542bb768","tenant_id":"acme_insurance","status":"scored","fraud_score":0.42,"model_version":"stub-v0"}
```
Confirms the panel renders plain (non-Avro) JSON cleanly, and that
`correlation_id`/`claim_id` in the Kafka message match the REST response
from the same TC-01 run exactly. ✅

Previously verified 2026-09-11 via a throwaway `kafka-go` consumer
(host-side, since `docker exec`-ing `rpk` into the broker container hits
the advertised-listener gotcha documented in RUNBOOK.md's
Troubleshooting — `rpk` inside the container can't resolve its own
`localhost:19092`) rather than the console UI:
```
partition=0 offset=0 key=acme_insurance value={"claim_id":"clm-kafka-verify-1","correlation_id":"f69262a450328a7ea56d9c90d56f172b","tenant_id":"acme_insurance","status":"scored","fraud_score":0.42,"model_version":"stub-v0"}
partition=0 offset=1 key=acme_insurance value={"claim_id":"clm-kafka-verify-2","correlation_id":"abd3ece941f4e56d5edb0bb27dcc6746","tenant_id":"acme_insurance","status":"scored","fraud_score":0.42,"model_version":"stub-v0"}
```

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

---

## 2026-09-13 — Protocol field added

TC-01's **Endpoint** line (`POST /api/v1/proxy` → forwards to `POST
/v1/claims`) read ambiguously — not obviously REST vs. gRPC at a glance,
despite the section header saying "REST." Fixed by promoting protocol to
its own first-class field: every test case now leads with **Protocol**
(`REST` / `gRPC` / `Kafka`), ahead of **Endpoint**. Six fields now, not
five (see "How to read each test case," updated to match).

Doing that put a **Protocol** field and the pre-existing **Proto** field
(which `.proto` file to upload) right next to each other — similar enough
names to recreate the exact confusion this was fixing. Renamed the latter
to **Proto file** throughout to keep them visually and semantically
distinct.

---

## 2026-09-14 — TC-02b, TC-03b added; correlation-ID gap found

Formalized two negative test cases that had been sitting as inline notes
rather than real entries: **TC-02b** (`GetTenant` with a typo'd
`tenant_id` — clean `NotFound`, from TC-02's 2026-09-11 ad-hoc test) and
**TC-03b** (`GetDagVersion` with `product` omitted entirely — also a
clean `NotFound`, `product` defaulting to `""` rather than the request
being rejected at the framing level). Also fixed TC-03's expected
response, which had gone stale: `{"version": 2}` → `{"version": 3}`,
matching `config/tenants.yaml`'s `dag_versions.auto` after the Kafka
publish work bumped it — the test case just hadn't been updated to match.

While working through TC-03b, found that **`tenant-config-svc` logs
nothing at all, on any path, success or error** (confirmed by reading
`internal/server/tenantconfig.go` directly — zero `log.Printf` calls in
either RPC handler). Traced this back further: it's not just missing log
statements — `tenant_config.proto`'s request messages have **no
`correlation_id` field**, unlike every sibling service's proto, and
neither caller (ClaimsGateway's `GetTenant`, Orchestration's
`GetDagVersion`) threads a correlation ID through despite having one in
scope. Confirmed the rest of the call graph (Orchestration → enrichment
services → Kafka) already propagates `correlation_id` correctly via the
established dual pattern (`x-correlation-id` gRPC metadata + an explicit
payload field) — `tenant-config-svc` is the one actual broken link, not
just an unused one.

**Decision**: fixing this (the proto field + threading) and the broader
fix (a `grpc.UnaryServerInterceptor`-style uniform logger across every
service, rather than more hand-written `log.Printf` calls) are both
explicitly deferred to DESIGN.md §11's observability/OpenTelemetry layer,
not done piecemeal now — recorded in [[current-progress]] (Claude's
memory) so it isn't rediscovered from scratch when that layer starts.
