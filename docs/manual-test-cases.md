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
`body.fraud_score: 0.05`, `body.model_version: "rules-v1"` (was `0.42`/
`"stub-v0"` before DECISIONS.md #16 — Model Service now returns a real,
deterministic score instead of a hardcoded one; `0.05` is what this
specific claim's inputs compute to, not an arbitrary new constant — see
`internal/scoring/scoring.go`).

**Last verified**: 2026-09-18 — re-run live against the real `rules-v1`
scoring formula (raw `curl` against ClaimsGateway directly, not the
console UI this pass): `200`, `1.73s`, `status:"scored"`,
`fraud_score:0.05`, `model_version:"rules-v1"`,
`normalized_address:"123 MAIN ST, SPRINGFIELD, IL, 62704, US"`. ✅ Exact
match to this test case's documented expected response — first re-run
since DECISIONS.md #16 shipped. (Prior runs below predate #16 and are
kept verbatim as historical record: 2026-09-13 — `200 OK`, `1045ms`,
`status:"scored"`, `fraud_score:0.42`, `normalized_address:"123 MAIN ST,
SPRINGFIELD, IL, 62704, US"`, via the actual browser UI, correct for the
stub in place at the time. Previously verified 2026-09-11: same result
shape, via UI for the first time that session — see that run note below.
2026-09-10: `durationMs:524`, same shape.)

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

**Worth knowing (stale as of the observability/resilience pass, kept for
history — see DECISIONS.md #17)**: this request used to produce **no
server-side log line at all**, success or error — same cause as TC-03b's
note (`tenant-config-svc` had zero `log.Printf` calls in either RPC
handler). That's fixed now: the server logs a `WARN "get tenant: not
found"` line via the shared structured logger, and `x-correlation-id`
metadata now actually reaches this service (previously it never did) —
expect `logs/tenant-config-svc.log` to have a line for this request the
next time this test case is run, not stay empty.

**Last verified**: 2026-09-14 — re-run live via the console's gRPC panel
with a different unknown `tenant_id` (`"fake_insurance_insurance-tenant"`),
got the expected `NotFound` error, and confirmed live via a tailed
`tenant-config-svc.log` panel that nothing was written to the log ("No
lines yet"). ✅ **Predates DECISIONS.md #17** — the empty-log observation
was correct at the time but no longer holds; not yet re-run since the
logging fix landed.

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

**Expected response**: `{"version": 4}` — matches
`config/tenants.yaml`'s `dag_versions.auto` (bumped 3→4 when
`max_retries` was added to `claim.fnol.yaml`'s enrichment nodes — see
DECISIONS.md #17).

**Last verified**: 2026-09-14 — re-run live via the console's gRPC panel,
`{"version":3}`, matching the corrected expected value exactly at the
time. ✅ **Stale** — predates the 3→4 bump above, not yet re-run.
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

**Worth knowing (stale as of the observability/resilience pass, kept for
history — see DECISIONS.md #17)**: this request used to produce **no
server-side log line at all**, success or error — `tenant-config-svc`
had zero `log.Printf` calls in either RPC handler (unlike ClaimsGateway,
which at least logged its error branches). Fixed now: the server logs a
`WARN "get dag version: not found"` line. See the 2026-09-14 note below
for the original discovery of this gap, and DECISIONS.md #17 for the fix.

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

**Last verified**: 2026-09-15 — re-run live via the console's gRPC panel,
`2ms`, exact match:
`{"normalizedAddress":"123 MAIN ST, SPRINGFIELD, IL, 62704, US",
"line1":"123 MAIN ST","city":"SPRINGFIELD","state":"IL",
"postalCode":"62704","country":"US","valid":true}`. ✅ (Previously
verified 2026-09-11, same result shape.)

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

**Last verified**: 2026-09-18 — re-run live via a throwaway gRPC client
(direct RPC, not the console UI this pass):
`claimant_id_hash:"7b9d77204ff21b167f8936bf0deefef40eee95574b6c9432fa890e5b6135065f"`
— byte-for-byte identical to both the 2026-09-11 and 2026-09-10 runs,
confirming the hash is still deterministic across sessions. ✅ (This
service is unaffected by DECISIONS.md #16 — hashing logic, not scoring —
so this re-run is a staleness check, not a formula-change check.)

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

**Last verified**: 2026-09-15 — re-run live via the console's gRPC panel,
`7ms`, exact match: `{"found":true,"status":"active","coverageType":"full"}`.
✅ (Previously verified 2026-09-10, same result.)

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

**Last verified**: 2026-09-15 — re-run live via the console's gRPC panel
(saved as "policyLookupService - Unknown Policy"), `2ms`,
`{"status":"unknown"}`, no error. ✅ Run with `policy_number: "POL"`
rather than the documented `"POL-BOGUS-999"` — still a valid run of this
test case, since the point is *any* unconfigured policy number returns
`status:"unknown"` without erroring, not that specific string; both
values are equally "not in the fixture map" (see
`services/policy-lookup-svc/.../PolicyLookup.java`'s `KNOWN_POLICIES`).
(Previously verified 2026-09-11 — same result. 2026-09-10 run had a
`response`/`durationMs` wrapper the UI apparently doesn't always render;
not a regression, `status:"unknown"` held both times.)

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

**Expected response**: `{"score":0.50, "modelVersion":"rules-v1"}` (was
`{"score":0.42, "modelVersion":"stub-v0"}` before DECISIONS.md #16 —
Model Service is now a real deterministic formula, not a hardcoded
value; real ONNX/ML inference is still pending). This payload's
`features` only sets `tenant_id` — none of `policy_status`/
`policy_coverage_type`/`address_valid` are present, so every one of the
formula's defaults applies (the riskiest case for each, per DESIGN.md
§9's "trained-in defaults, not nulls"): `0.05` base + `0.30` (no
`policy_status`) + `0.05` (no `policy_coverage_type`) + `0.10` (no
`address_valid`) = `0.50`. Incidentally a good regression check for the
all-defaults path — see `internal/scoring/scoring_test.go`'s equivalent
case.

**Last verified**: 2026-09-18 — re-run live against the real `rules-v1`
formula via a throwaway gRPC client (direct RPC, not the console UI this
pass): `score:0.5, model_version:"rules-v1"`. ✅ Exact match to the
all-defaults calculation documented above — first re-run since
DECISIONS.md #16 shipped. (2026-09-15 run below predates #16, kept
verbatim as historical record: re-run live via the console's gRPC panel,
`0ms`, exact match at the time: `{"score":0.42,"modelVersion":"stub-v0"}`,
correct for the stub in place then. Previously verified 2026-09-10, same
result; not re-run 2026-09-11.)

---

## TC-09 — OrchestrationService: `ProcessClaim` via direct gRPC

**Now a real pass/fail test** — both console-side blockers that used to
make this "known limitation only" are resolved as of 2026-09-15/16:
multi-file proto upload (`orchestration.proto` imports
`claims/v1/claims.proto`) and per-call gRPC metadata (`claims.v1.ClaimEvent`
deliberately excludes `tenant_id` from the payload — it travels as
`x-tenant-id` gRPC metadata, injected by ClaimsGateway's interceptor, see
[DESIGN.md](../DESIGN.md) §8). Confirmed by reading that project's
current source directly (`GrpcPanel.tsx`'s `fileRows`/`grpcFiles.ts` for
multi-file; `InvokeRequest.Metadata map[string][]string` +
`invoker.go`'s `metadata.NewOutgoingContext` for headers, both now real,
not just spec'd) and **by actually invoking this exact RPC through the
raw API and getting a genuine scored response back**, not an error — see
Last verified below. This is the first version of this test case that
exercises OrchestrationService's real logic directly via gRPC, not just
indirectly through TC-01's REST path.

**Protocol**: gRPC

**Endpoint**: gRPC `orchestration.v1.OrchestrationService/ProcessClaim`

**Port**: `localhost:9091`

**Proto file**: `orchestration/v1/orchestration.proto` +
`claims/v1/claims.proto` — upload both directly in the console's gRPC
panel (multi-file support: add both file rows, set
`orchestration/v1/orchestration.proto` as the entry file).

**Metadata** (new panel section, "Metadata (sent as gRPC headers, not
part of the request body)" — add two rows):
```
x-tenant-id: acme_insurance
x-correlation-id: tc09-test-1
```

**Payload** (the Request field):
```json
{"claim": {"claim_id": "clm-console-002", "product": "auto", "event_type": "claim.fnol", "policy_number": "POL-123456", "claimant_name": "Jane Doe", "raw_address": "123 Main St, Springfield, IL 62704"}}
```

**Console UI steps** (this is the first test case in this file to use
either the multi-file or metadata UI, so spelled out precisely rather
than assumed — GRPC tab):
1. First file row: paste
   [proto/orchestration/v1/orchestration.proto](../proto/orchestration/v1/orchestration.proto)'s
   contents into the textarea, set its filename field to
   `orchestration/v1/orchestration.proto`, and select its **Entry file**
   radio button.
2. Click **+ Add file**. In the new second row, paste
   [proto/claims/v1/claims.proto](../proto/claims/v1/claims.proto)'s
   contents, filename `claims/v1/claims.proto`. Leave its **Entry file**
   radio unselected (row 1 stays the entry file).
3. Click **Parse**.
4. **Service** dropdown → `orchestration.v1.OrchestrationService`,
   **Method** → `ProcessClaim`, **Target** → `localhost:9091`.
5. Under **Metadata**, click **+ Add metadata** twice. Row 1: key
   `x-tenant-id`, value `acme_insurance`. Row 2: key `x-correlation-id`,
   value `tc09-test-1` (or any string — it just round-trips into the
   response).
6. Paste the **Payload** JSON above into the Request field.
7. Click **Invoke**.

**Raw-API equivalent** (if driving via curl instead of the UI — same
idea as every other test case's raw-API note; run from the repo root):
```sh
jq -n \
  --arg entry "orchestration/v1/orchestration.proto" \
  --rawfile orch "proto/orchestration/v1/orchestration.proto" \
  --rawfile claims "proto/claims/v1/claims.proto" \
  '{entryFile: $entry, files: {"orchestration/v1/orchestration.proto": $orch, "claims/v1/claims.proto": $claims}}' \
  > /tmp/parse_request.json

HASH=$(curl -s -X POST http://localhost:8090/api/v1/grpc/parse \
  -H "Content-Type: application/json" \
  -d @/tmp/parse_request.json | jq -r .hash)

curl -s -X POST http://localhost:8090/api/v1/grpc/invoke \
  -H "Content-Type: application/json" \
  -d "{
    \"hash\": \"$HASH\",
    \"service\": \"orchestration.v1.OrchestrationService\",
    \"method\": \"ProcessClaim\",
    \"target\": \"localhost:9091\",
    \"metadata\": {\"x-tenant-id\": [\"acme_insurance\"], \"x-correlation-id\": [\"tc09-test-1\"]},
    \"request\": {\"claim\": {\"claim_id\": \"clm-console-002\", \"product\": \"auto\", \"event_type\": \"claim.fnol\", \"policy_number\": \"POL-123456\", \"claimant_name\": \"Jane Doe\", \"raw_address\": \"123 Main St, Springfield, IL 62704\"}}
  }"
```

**Expected response**: a real scored result, matching TC-01's shape —
`status:"scored"`, `fraud_score:0.05`, `model_version:"rules-v1"` for
this specific claim's inputs (same reasoning as TC-01's note).

**Last verified**: 2026-09-16 — **through the actual browser UI**,
following this test case's own Console UI steps above exactly (2 file
rows, both metadata rows, `ProcessClaim`): `1008ms`,
```json
{"claimId":"clm-console-002","correlationId":"tc09-test-1","status":"scored","normalizedAddress":"123 MAIN ST, SPRINGFIELD, IL, 62704, US","fraudScore":0.05,"modelVersion":"rules-v1"}
```
Exact match. ✅ Kafka panel also showed the resulting message land on
`claims.realtime` in the same session (key `acme_insurance`,
`correlation_id` matching). This is the first time both
OrchestrationService's real DAG logic *and* the console's own multi-file
+ metadata UI have been exercised together, through the real browser UI,
in one confirmed pass — not the raw API, and not inferred from TC-01.

(Earlier the same day: first attempt via the raw API returned the old
`NotFound: tenant ""` error even with `metadata` set, traced to a stale
console backend process — started 2026-09-13, before the metadata
feature existed in source. Restarted it, raw API succeeded, then the UI
run above confirmed it end-to-end. If you ever hit `NotFound: tenant ""`
again despite setting metadata correctly, check whether the console
backend process predates your last build in that project first.)

**History, kept for context** (this test case was "known limitation
only" from 2026-09-10 through 2026-09-15 — see git history on this file
for those entries if the full story matters): originally
`orchestration.proto`'s import wasn't parseable via the frontend's
single-file upload; multi-file support landed 2026-09-15, confirmed via
that project's `skills/grpc.md` (maintainer re-ran this exact proto pair
through headless Chromium). The remaining metadata gap was tracked as
"Feature 11 — gRPC Invoke Metadata" in that project's `docs/ROADMAP.md`
and implemented by 2026-09-16, closing this out completely.

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
just submitted, `status:"scored"`, `fraud_score:0.05`,
`model_version:"rules-v1"` (was `0.42`/`"stub-v0"` before DECISIONS.md
#16 — see TC-01's note for why `0.05` specifically).

**Last verified**: 2026-09-18 — re-run live against the real `rules-v1`
formula via a throwaway `kafka-go` consumer (host-side, reading from
offset 0), not the console UI this pass: submitted a fresh claim
(`clm-tc10-rerun`) via `POST /v1/claims`, then confirmed it landed on the
broker at offset 15:
```
offset=15 key=acme_insurance value={"claim_id":"clm-tc10-rerun","correlation_id":"75d9aa237af85862f64788a2b40e211a","tenant_id":"acme_insurance","status":"scored","fraud_score":0.05,"model_version":"rules-v1"}
```
`correlation_id` matches the same claim's `POST /v1/claims` response
exactly. ✅ First re-run since DECISIONS.md #16 shipped — `fraud_score`
is now `0.05`/`rules-v1` instead of the old stub values below.

Prior runs kept verbatim as historical record, all predating #16:

2026-09-13 — **first time actually clicked through in
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
from the same TC-01 run exactly. ✅ **Both runs below predate
DECISIONS.md #16** — `fraud_score:0.42`/`model_version:"stub-v0"` were
correct for the stub in place at the time; not yet re-run against the
real scoring formula, due next pass (kept verbatim as an accurate
historical record, not updated in place).

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

## TC-11 — Orchestration: a stopped downstream service degrades within
     bounded time, not indefinitely

New as of the observability/resilience pass (DECISIONS.md #17): proves
`Node.TimeoutMs` — parsed from `claim.fnol.yaml` since the walking
skeleton, but never actually enforced until now — really bounds a
downstream call, and that `max_retries: 1` really retries once before
`on_failure` takes over. Not a new protocol/endpoint (reuses TC-01's
REST path); the point is *timing*, not the response shape.

**Protocol**: REST (same endpoint as TC-01 — `POST /v1/claims` via
ClaimsGateway, `localhost:8080`)

**Endpoint**: `POST /v1/claims`

**Port**: `localhost:8080`

**Payload**: same as TC-01's.

**Steps**:
1. With the full stack up, stop `policy-lookup-svc` only (Ctrl+C its
   terminal, or kill its PID if started via `scripts/start-all.sh` — see
   RUNBOOK.md step 6 for how to find it, e.g. `Get-NetTCPConnection
   -LocalPort 9096`). Leave the other 6 services + broker running.
2. Submit a claim (TC-01's payload works as-is) and time the request.
3. Check `logs/orchestration-service.log` for the `policy_lookup` node's
   outcome.

**Expected response**: `200`, completing in roughly bounded time — with
`policy_lookup`'s `timeout_ms: 40` and `max_retries: 1`, each of the (up
to) 2 attempts is capped at 40ms, so the node itself resolves in well
under 200ms regardless of how long a truly-down service would otherwise
be dialed against (gRPC's own connection-refused failure is typically
fast, but the *timeout enforcement* is what's being proven, not just
that a downed service errors quickly — see DECISIONS.md #17). `status:
"degraded"` (policy_lookup's `on_failure: degrade`), `fraud_score` still
present (model scoring still runs on the remaining features).
`logs/orchestration-service.log` shows two attempts at the `policy_lookup`
node (the retry) before a `"msg":"dag node failed, continuing"` line with
`"node":"policy_lookup","on_failure":"degrade"`.

**Last verified**: 2026-09-23 — stopped `policy-lookup-svc` (`Stop-Process`
on its PID), submitted a fresh claim: `200`, **39ms total**,
`status:"degraded"`, `fraud_score:0.4`. ✅ `logs/orchestration-service.log`
showed exactly 2 attempts at `policy_lookup` (both fast
`Unavailable`/connection-refused errors, ~0-1ms each — the target process
was gone, not slow), then `"dag node failed, continuing"` with
`"node":"policy_lookup","on_failure":"degrade"`, then `model_score` and
`publish` both completed normally. Confirms `max_retries` and the
on_failure policy both fire correctly, and — the actual point of this
test case — the whole request resolved in well under a second instead of
hanging. (Restarted `policy-lookup-svc` afterward to restore the full
stack; confirmed `status:"scored"` again once its gRPC client connection
reconnected — see the note on connection backoff below.)

**Note on the two real bugs this test case's own dev pass surfaced and
fixed** (kept here since they were found *while building* the
timeout/retry enforcement this test case exercises, not by running TC-11
itself): (1) `grpc.NewClient` dials lazily on the first real RPC, so a
cold connection's handshake used to compete with a node's `timeout_ms`
on the very first request after a fresh restart — fixed by
`telemetry.WarmUp` blocking (up to 5s) for the connection to reach
`Ready` before the service starts serving. (2) kafka-go's `Writer`
defaults to a 1-second `BatchTimeout` — every historical "~1s" TC-01/
TC-10 duration in this file was actually that default batch window, not
JVM/network overhead; fixed by setting `BatchTimeout: 10ms` on the
writer, since `Publish()` sends one message at a time, not a real
producer batch. Both are documented in DECISIONS.md #17. A downstream
service that goes down and comes back **after** orchestration-service's
own client already connected still takes a few seconds to recover (gRPC
client reconnect backoff, not something `WarmUp` — a one-time,
dial-time-only check — addresses); expected, not a bug.

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
