# Build Decisions

A running log of decisions made while building this platform — the *why*
behind the build order and the simplifications, not just the *what*
(DESIGN.md covers the target architecture; this covers how we're actually
getting there and what's been deliberately deferred at each step). Kept
up to date as new layers land — see README.md's Status section for where
things currently stand.

## 1. Walking skeleton before any single service

**Decision:** build a thin, real request flowing end-to-end
(ClaimsGateway → Orchestration → one enrichment service → Model Service →
publish) before deepening *any* one piece — including the architecturally
"interesting" parts like the DAG engine or Tenant Config Service.

**Why:** a walking skeleton surfaces integration problems (protobuf
contracts, REST↔gRPC translation, Go↔Java gRPC interop) immediately, while
they're still cheap to debug — rather than building the DAG engine in
isolation and discovering its contract is wrong once a real consumer shows
up.

**Rejected alternative:** build the "hard part" (DAG engine / Tenant Config
Service) first, since it's the most architecturally significant piece.
Rejected because it's the piece most likely to need revision once it has a
real caller, and revising it in isolation (with no consumer to validate
against) risks guessing wrong.

This isn't a one-time decision — it's the general strategy applied at each
subsequent layer (see #8).

## 2. Address Normalization picked first among enrichment services

**Decision:** of the enrichment services in DESIGN.md's example DAG
(claimant ID hashing, address normalization, policy lookup), address
normalization went first.

**Why:** it's pure input→output transformation — no PII hashing, no
encryption, no external policy-system dependency. It proves the
gRPC contract + Spring Boot service structure + Orchestration↔enrichment
call pattern without also having to solve encryption/PII handling in the
same pass.

## 3. Model Service and Kafka publish stubbed first

**Decision:** Model Service returns a fixed score (`0.42`, `model_version:
stub-v0`); the publish step logs instead of writing to Kafka.

**Why:** the walking skeleton's goal is proving the *shape* of the request
flow (who calls whom, in what order, with what contract) — real inference
(ONNX/Triton) and real Kafka publishing are orthogonal concerns that can be
swapped in later without changing anything upstream, since callers only
depend on the gRPC contract, not the implementation behind it.

## 4. `tenant_id` and `correlation_id` travel as gRPC metadata, not payload

**Decision:** neither field is part of any protobuf message body (see
`claims.proto`'s `ClaimEvent`). Both are injected as gRPC metadata
(`x-tenant-id`, `x-correlation-id`) — `tenant_id` by ClaimsGateway after
resolving it, `correlation_id` generated at ingress and propagated the same
way (also duplicated into `ClaimEvent.correlation_id` for convenience, since
it's useful in logs/response payloads too).

**Why:** matches DESIGN.md §8 — every downstream service reads tenant
context via interceptor/metadata, never trusts a client-supplied
`tenant_id` field. Keeping it out of the message payload entirely makes it
structurally impossible for a client (or a bug) to spoof which tenant a
claim belongs to via the request body.

## 5. Isolation model: Bridge (hybrid multi-tenancy)

**Decision:** shared compute for ClaimsGateway/Orchestration/enrichment
services, routed and behaviorally customized per tenant via DAG config; but
isolated storage for anything sensitive (per-tenant KMS keys, field-level
PII encryption, Data Lake partitioned by `tenant_id`).

**Rejected alternatives:** full **silo** (dedicated infra per carrier — too
much duplication for a portfolio project, doesn't teach multi-tenancy
patterns); pure **pool** (shared everything with just a `tenant_id` column —
under-represents real data-segregation requirements in insurance).

See DESIGN.md §5/§6 for the full reasoning.

## 6. Java services build on Maven, not Gradle

**Decision:** every Java service (address-normalization-svc and future
enrichment services) uses `pom.xml`/Maven.

**Why:** explicit project constraint, not derived from the architecture —
kept consistent across all Java services regardless of individual service
preferences.

## 7. Each Go service is its own module, tied together by `go.work`

**Decision:** `claims-gateway`, `orchestration-service`, `model-service`,
`tenant-config-svc`, and the generated `proto/gen/go` stubs are each a
separate Go module (own `go.mod`/`go.sum`), joined only via the repo-root
`go.work` for local development.

**Why:** each is an independently deployable unit — a separate `go.mod` per
service keeps their dependency graphs (and eventual container images)
independent, while `go.work` is purely a local-dev convenience letting
`go build`/gopls resolve cross-module imports (e.g. generated protobuf
types) without needing `replace` directives or a published module version
for code that only exists in this monorepo.

**Gotcha hit:** `go.work`'s `go` directive requires the *locally installed*
Go SDK to actually meet that version — `go build` masks a stale local SDK
via `GOTOOLCHAIN=auto`, but `gopls` doesn't, so IDE navigation breaks
silently until the local SDK is actually upgraded. Not a design decision,
but worth knowing if this bites again after a future version bump.

## 8. Layer 2a (Tenant Config Service + DAG loading) is *also* a thin slice

**Decision:** rather than building DESIGN.md §6.1/§7 to full spec in one
pass (Postgres, transactional outbox, Kafka cache invalidation, JSON Schema
validation, API-key auth), this layer proves the *service boundaries and
config-driven shape* first: a real gRPC Tenant Config Service backed by a
local YAML file, and a config-driven DAG executor in Orchestration — both
loading config from disk/memory with no caching or hot-reload yet.

**Why:** #1's reasoning applies recursively — prove the contract (a real
service boundary, a real config-driven DAG shape) before investing in the
storage/caching/invalidation machinery behind it. Concretely deferred to a
later pass:
- Postgres-backed tenant store + transactional outbox
- Kafka-based cache invalidation (`tenant-config-events`, `dag-config.updated`)
- JSON Schema validation on DAG YAML
- Per-caller TTL caching (every call re-reads from disk/memory today)
- API-key-based `ResolveTenant` (still a hardcoded default id, see #9)

## 9. `GetTenant(tenant_id)` stands in for `ResolveTenant(api_key_hash)`

**Decision:** ClaimsGateway still resolves "which tenant" via a hardcoded
default id (no client-supplied `tenant_id`, no auth) — but that id is now a
lookup key into a real Tenant Config Service call, which returns real
status data used to gate the request (`403` if not `active`), rather than
being assumed active.

**Why:** proves the *status-gating path* for real — including a live
`403` response, verified against a genuinely `suspended` tenant seeded in
`config/tenants.yaml` — without needing to first build API-key
issuance/hashing/auth middleware, which is orthogonal to whether the gating
logic itself works.

## 10. DAG executor is staged, not a general topological sort

**Decision:** Orchestration's new config-driven executor recognizes exactly
three fixed stages — an "enrichment" group (nodes with no `depends_on`, able
to run concurrently), nodes depending on that group, then nodes depending on
those — rather than resolving arbitrary `depends_on` graphs.

**Why:** every DAG shape in DESIGN.md's example (and everything planned
through layer 2b's `claimant_id_hash`/`policy_lookup` — both join the
enrichment group, neither depends on the other) fits this shape. A general
topo-sort is real, non-trivial work that has no current DAG to justify it;
building it now would be solving a problem this repo doesn't have yet.

## 11. `config/` lives at repo root, not nested in a service

**Decision:** `config/tenants.yaml` and `config/dag/{tenant}/{product}/
{event_type}.yaml` sit at the repo root alongside `proto/`.

**Why:** mirrors `proto/`'s existing rationale — both are shared, single
sources of truth read by multiple services (tenant-config-svc reads
`tenants.yaml`; Orchestration reads the DAG YAML directly from disk), so
neither belongs inside any one service's directory.

## 12. Claimant ID Hashing uses a static salt, not per-tenant KMS keys

**Decision:** `ClaimantIdHasher` (claimant-id-hashing-svc) hashes
`claimant_name` with SHA-256 and one static salt baked into the binary,
not a per-tenant key derived from KMS.

**Why:** #1/#8's reasoning again — this proves the actual contract that
matters (raw PII never reaches Model Service or Kafka; claims from the same
claimant hash identically for fraud-pattern matching) without first
building KMS integration, which is a meaningful chunk of infrastructure
(DESIGN.md §12) orthogonal to whether the hashing *behavior* is correct.
Also: `claims.v1.ClaimEvent` doesn't carry a richer PII identifier
(SSN/DOB) yet — `claimant_name` is what's available, and is itself
case/whitespace-normalized before hashing so `"Jane Doe"` and `"jane doe"`
resolve to the same claimant.

**Deferred:** per-tenant KMS-derived salts/keys, a real PII identifier
field in `ClaimEvent`.

## 13. Policy Lookup is fixture-backed, not a real policy-system integration

**Decision:** `PolicyLookup` (policy-lookup-svc) resolves `policy_number`
against a small hardcoded `Map` of known policies, not a call to a real
policy-admin system.

**Why:** same reasoning as Model Service's stub (#3) and Tenant Config
Service's local-YAML store (#8) — proves the enrichment contract (a
found/not-found policy, status, coverage type feeding the model's feature
map) without a policy-admin-system integration that doesn't exist yet in
this portfolio project. `on_failure: degrade` (not `fail_fast`) reflects
that a missing/unknown policy is a legitimate, expected outcome, not
grounds to abort the claim.

## 14. Model Service features carry the claimant hash, never raw PII

**Decision:** once `claimant_id_hash` succeeds, the feature map
Orchestration sends to Model Service uses `claimant_id_hash` — raw
`claimant_name` is dropped from the outgoing feature map entirely, not
merely supplemented by the hash.

**Why:** the entire point of hashing the claimant identifier (DESIGN.md §4
item 3) is keeping raw PII from propagating past the one service that's
supposed to touch it. Sending both the hash *and* the raw name downstream
would defeat that. This is safe specifically because `claimant_id_hash`'s
`on_failure` is `fail_fast` (DESIGN.md §6.1's example) — by the time
Orchestration reaches the feature-map-building stage, the hash is
guaranteed to exist, so there's no case where raw PII is the only thing
available to send.

## 15. Real Kafka publish: JSON to a Redpanda broker, not Protobuf +
    schema registry yet

**Decision:** the `kafka-publisher` DAG node is now a real publish
(`internal/kafka`, `github.com/segmentio/kafka-go`) to `claims.realtime`
(DESIGN.md §10), replacing the log-only stub. Messages are plain
JSON-encoded `ScoredClaimEvent` structs, keyed by `tenant_id` for
partitioning. The broker itself is Redpanda via `docker-compose.yml`
(Kafka-API compatible, same choice already proven out in the separate
all-in-one-end-to-end-test-console project), exposed on host port
**`19092`**, not Kafka's conventional `9092` — that port already belongs
to address-normalization-svc's own gRPC server in this repo, and
separately to the test-console project's own Redpanda broker; `19092`
avoids colliding with either. Also added `on_failure: skip` to the
`publish` node (`claim.fnol.yaml` v3) — the score is already computed by
the time this stage runs, so a broker outage should degrade the response
(client still gets a score), not fail the whole request.

**Why:** proves the actual contract that matters — a scored claim
reliably reaches a real broker on the real target topic, correctly
partitioned — without first building the full DESIGN.md §10 messaging
stack (Protobuf wire format, schema registry, topic provisioning as its
own concern). Same "thin slice first" reasoning as #1/#3/#8/#12/#13.
JSON specifically (not raw Protobuf bytes) also keeps the message
human-readable in the test console's Kafka panel without needing Avro/
schema-registry decode support it doesn't have — directly useful for
`docs/manual-test-cases.md`'s console-driven verification.

**Deferred:** Protobuf wire format + Confluent schema registry
integration for `claims.realtime` (DESIGN.md's target messaging
convention), `claims.historical` (Batch Transform's topic — no batch flow
yet at all), `dag-config.updated` cache
invalidation (§6.1, tracked under #8's deferred scope), topic
provisioning/retention as an explicit ops concern (currently relies on
`AllowAutoTopicCreation`).
