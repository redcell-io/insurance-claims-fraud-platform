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

## 16. Model Service: a real deterministic formula, not ONNX yet

**Decision:** `internal/scoring.Score` replaces the old hardcoded stub
(`score: 0.42, model_version: "stub-v0"`, identical for every claim
regardless of input) with a real, hand-weighted formula over the feature
map Orchestration assembles: `policy_status` (`active` +0.00, `lapsed`
+0.15, `cancelled` +0.25, unknown/not-found +0.30), `policy_coverage_type`
empty +0.05, `address_valid` not `true` +0.10, from a `0.05` base,
clamped to `[0, 1]`. `model_version` is now `"rules-v1"`. Each triggered
rule is logged server-side with `correlation_id` (`internal/server/model.go`)
— the first real log output this service has ever produced, and DESIGN.md
§9's "feature-level explanations" goal in spirit, without a proto/API
change this pass.

Needed one new signal that didn't exist yet: `address_valid` wasn't in
Orchestration's feature map (only `normalized_address`, the string, was).
Added by capturing `NormalizeResponse.valid` in
`internal/dag/executor.go`'s Stage 1, defaulting to `false` (not omitted)
when the node is skipped/degraded — DESIGN.md §9: "trained-in defaults,
not nulls," a claim this service can't confidently verify the address for
should read as unverified, not silently absent.

**Why:** proves the score actually *responds* to real enrichment signals
— submit a claim against an unknown policy number and it visibly scores
higher, submit the standard smoke-test claim and it's a clean, reproducible
`0.05` — without first building ONNX Runtime integration, a trained model
artifact, or model-version hot-swapping. Same "thin slice first" reasoning
as #1/#3/#8/#12/#13/#15. Stays in **Go**, no language switch to Java:
this is a formula, not a model-serving runtime, so there's no reason to
pull in ONNX Runtime or match the other enrichment services' language for
it.

**Rejected for this pass:** exposing `explanations` as a new
`ScoreResponse` field (would need a proto change + regen for a thin slice
that's explicitly not the real API contract yet — logging is enough to
prove the reasoning is real); a Go ONNX Runtime binding
(`github.com/yalue/onnxruntime_go`) instead of a formula (still would've
needed a trained model artifact from somewhere — doesn't avoid the real
cost, just moves it).

**Deferred:** real ONNX Runtime (or TorchServe/Triton) inference, a
trained model artifact (versioned, per DESIGN.md §9, in S3), hot-swappable
`model_version` resolved per tenant/product (same pattern as DAG
versions), feature-level explanations actually exposed via the API rather
than just logged.

## 17. Observability/resilience layer: structured logging + DAG
    timeout/retry, not the full DESIGN.md §11 stack

**Decision:** this pass covers three things only: (1) a new shared Go
module, `pkg/telemetry`, providing JSON logging (`log/slog`) and real
`grpc.UnaryServerInterceptor`/`UnaryClientInterceptor` implementations —
replacing every hand-rolled `log.Printf` and the ad-hoc metadata
read/write code that used to live in `orchestration-service`'s server
package and `claims-gateway`'s client package (both had comments
admitting they were stand-ins for this); (2) the matching fix on the
Java side — `logstash-logback-encoder` + a per-service
`logback-spring.xml` (duplicated 3×, same pattern as
`GrpcServerLifecycle.java`) plus real SLF4J + MDC logging in all three
enrichment services' gRPC handlers, which previously had zero log calls
at all; (3) DAG-level resilience in `orchestration-service/internal/dag`
— `Node.TimeoutMs` (parsed from YAML since the walking skeleton but
never enforced) is now wrapped around every downstream call via
`context.WithTimeout`, and a new `Node.MaxRetries` field adds simple
retry-with-fixed-backoff on transient failures, both before the existing
`on_failure` (fail_fast/skip/degrade) policy applies.

Not attempted this pass: OpenTelemetry tracing + Jaeger, Prometheus
metrics/`/metrics` endpoints, Grafana dashboards, centralized log
shipping (Loki/ELK), circuit breakers (open/half-open state — this is
timeout+retry only), or mTLS (DESIGN.md §12, a security concern, not
observability).

**Why:** same "thin slice first" reasoning as #1/#3/#8/#12/#13/#15/#16 —
DESIGN.md §11's full target (tracing + metrics + log aggregation, all
across 7 services in 2 languages) is a multi-day effort on its own; this
pass instead closes the two concrete gaps that had been flagged
repeatedly across sessions without being picked up (tenant-config-svc's
missing correlation ID/zero logging, and `timeout_ms` being parsed but
never actually enforced) plus gives every service a consistent
structured-logging foundation, without committing to a tracing/metrics
backend before there's a second consumer of that data to justify it.

**Consistency choice:** `tenant-config-svc` now receives
`x-correlation-id` as gRPC metadata (via the new client interceptor
whenever the caller's context carries one), not a new field on
`tenant_config.proto`'s request messages — matches DECISIONS.md #4's
existing metadata-first principle for Go-to-Go calls, rather than adding
a second, inconsistent transport for the same data. The Java enrichment
services keep their existing pattern unchanged (`correlation_id` as an
explicit request-payload field, per DESIGN.md/DECISIONS.md #4's
carve-out for those calls) — they just log it now, via MDC, instead of
receiving it and doing nothing with it.

**Retry scope kept deliberately simple:** `callWithResilience` retries
on *any* error, not just gRPC codes that are actually safe to retry
(`Unavailable`, `DeadlineExceeded`) — a real implementation would
distinguish those from e.g. `InvalidArgument`, which retrying can't fix.
Fixed-delay backoff (20ms), not exponential/jittered. Both are
proportionate to this pass's scope; revisit if retries ever need to be
safe against non-idempotent downstream effects (none of today's
enrichment calls have any).

**Two real bugs this pass's live testing surfaced, both fixed, neither
hypothetical:**

1. **Cold gRPC connections competed with the new timeouts.**
   `grpc.NewClient` dials lazily, only on the first real RPC — fine when
   nothing times that call out, but with `timeout_ms` now enforced, a
   fresh restart's first request to each downstream service reliably
   blew its budget on TCP/HTTP2 handshake + (for the Java services) JVM
   cold-start alone, before any real work happened. Fixed by
   `telemetry.WarmUp` (`pkg/telemetry/warmup.go`): every `Dial*`
   function now calls `conn.Connect()` and blocks (up to 5s, logging a
   warning rather than failing on timeout) for the connection to reach
   `Ready` before the service starts serving traffic, so that cost is
   paid once at startup instead of on every service's very first real
   request.
2. **kafka-go's `Writer` defaults `BatchTimeout` to 1 second.** Every
   historical "~1 second" request duration recorded elsewhere in this
   repo's docs (TC-01, TC-10) wasn't JVM/network overhead — it was this
   default batch window, previously invisible because nothing timed the
   publish call out. The DAG's `publish` node's own `timeout_ms: 100`
   (present since the walking skeleton, just unenforced) made this
   visible immediately: every publish failed on its first attempt, no
   matter how healthy the broker was. Fixed by setting
   `BatchTimeout: 10ms` on the `kafka-go.Writer` in
   `internal/kafka/publisher.go` — `Publish()` sends one message per
   call, not a real producer batch, so there's no reason to hold it
   before flushing. Net effect: requests that used to take ~1-1.7s now
   complete in well under 100ms.

## 18. Prometheus RED metrics (tracing still deferred)

**Decision:** every service exposes a `/metrics` endpoint (Prometheus
exposition format) and records two RED metrics — `grpc_requests_total{method,
tenant_id, status}` and `grpc_request_duration_seconds{method, tenant_id}` —
for every gRPC call it handles or makes, via `pkg/telemetry`'s
`RecordRequest`, called from the existing `UnaryServerInterceptor`/
`UnaryClientInterceptor` (same two functions #17 added — this is additive
to code that already computes duration and knows method/error state, not
a new instrumentation surface). A new `prometheus` container (`prom/prometheus`,
`docker-compose.yml`) scrapes all 7 services' `/metrics` ports.

Picked from a short list of candidate next phases (batch flow, real ONNX
model, real tenant resolution, metrics+tracing) — this one continues
directly from #17, which explicitly listed Prometheus metrics under "Not
attempted this pass." Scoped to metrics only; OpenTelemetry tracing +
Jaeger is a deliberate follow-up pass, not bundled in here — same
"smaller, verifiable, one layer at a time" reasoning as every other
thin-slice decision in this file, rather than one larger combined change
touching every service's `main.go`/`pom.xml` twice as much before
anything is testable.

**No `service` label on the metrics.** Each service is scraped on its
own dedicated port, and Prometheus's own scrape `job` label (set once in
`config/prometheus.yml`) already distinguishes which service a sample
came from — adding an explicit `service` label would duplicate that same
information and, because `*slog.Logger` doesn't expose its own attached
attributes, would have required threading a service name through all 11
existing `UnaryServerInterceptor`/`UnaryClientInterceptor` call sites for
no new information. `rate(grpc_requests_total{job="orchestration-service"}[1m])`
queries identically to how a `service` label would. Revisit only if these
metrics are ever federated/aggregated somewhere downstream that drops
the `job` label.

**Java services' `/metrics`: hand-rolled JDK `HttpServer`, not Spring
Boot Actuator.** The three enrichment services are deliberately non-web
today — gRPC only, manual `SmartLifecycle` (`GrpcServerLifecycle.java`'s
own doc comment explains why: no `grpc-spring-boot-starter`, kept
consistent by hand across all three rather than extracted into a shared
starter). `spring-boot-starter-web` + Actuator would give
`/actuator/prometheus` for near-zero code, plus free bonus endpoints
(health, env, etc.), but pulls in an embedded Tomcat servlet container
for exactly one route — more baseline heap, more threads, slower
startup — multiplied across three JVMs that may end up co-located on one
modest VM (the planned Hostinger deployment for the offline/batch flow,
AWS later for real-time). It would also quietly undo the "non-web"
design those services currently have on purpose. Chose
`micrometer-registry-prometheus` (library only, Spring-BOM-managed
version, unlike grpc/protobuf which this repo pins explicitly since
those aren't Spring-managed) plus a new `MetricsServerLifecycle.java`
sibling to `GrpcServerLifecycle.java`, same `SmartLifecycle` pattern,
serving `/metrics` via `com.sun.net.httpserver.HttpServer` (JDK
built-in, zero new dependency). More code than Actuator would need, but
keeps the footprint minimal and the architecture consistent with what's
already there; Actuator can still be added later if its extra endpoints
turn out to be worth the cost once there's a concrete ops need for them.

**This choice is about startup/footprint cost, not per-request latency —
worth being precise about given this service's real-time SLA**
(`DESIGN.md` §3: "sub-300ms for a fraud score at claim submission," the
stated example target). `RecordRequest`'s actual work inside the request
path — one counter increment, one histogram observation — is exactly
what Prometheus client libraries are built to make safe on every
request; that cost is the same regardless of which HTTP framework
eventually serves `/metrics`. The `/metrics` endpoint itself runs on its
own port and its own thread(s), fully decoupled from the gRPC server's
request-handling threads, and is scraped by Prometheus roughly every
15s in the background — it is never on the path of an actual claim-
scoring request, Actuator or not. Where hand-rolled vs. Actuator
actually differs is baseline memory/thread footprint, which only matters
indirectly, via GC pressure under memory contention on a constrained
VM — a possible source of occasional latency-tail spikes, not a direct
hit to normal request latency. Don't conflate the two when revisiting
this: the SLA argument supports the lighter option, but it isn't really
an argument against Actuator on *request-latency* grounds specifically.

**Not attempted this pass** (still open, same as #17 left them):
OpenTelemetry tracing + Jaeger, Grafana dashboards, centralized log
shipping, circuit breakers, mTLS.

## 19. Containerized the full stack in a separate compose file, not by editing docker-compose.yml

Picked off the open list ahead of a VM deployment (Hostinger), scoped to
**demo-grade**: get all 7 services running as containers end-to-end, no
TLS/domain/CI yet. Deferred this long on purpose — Postgres, ONNX model
artifacts, batch flow, and OTel/Jaeger were all still open items that
would each add new entries to any compose/Dockerfile work, so doing
this once the service surface was momentarily stable (right after #18)
was cheaper than doing it twice.

**What shipped**: a `Dockerfile` per service (4 Go, multi-stage
`golang:1.25` → `gcr.io/distroless/static-debian12`; 3 Java, multi-stage
`eclipse-temurin:25-jdk` + hand-installed Maven 3.9.9 (from
`archive.apache.org`, not `dlcdn.apache.org` — the latter only mirrors
*current* releases, 404s on anything superseded) + a generated
`toolchains.xml` registering the image's own JDK as "temurin 25" →
`eclipse-temurin:25-jre`), a new `docker-compose.deploy.yml` wiring all
7 plus the broker and Prometheus onto one bridge network via compose
service-name DNS, and `config/prometheus.docker.yml` (service names
instead of `host.docker.internal`).

**Deliberately a separate compose file, not an edit to the existing
`docker-compose.yml`**: that file's broker+Prometheus back the
host-native dev loop (`scripts/start-all.sh`) that TC-01 through TC-12
are all verified against — changing it in place would invalidate that
verification history for no benefit. The two are mutually exclusive on
host ports (8080, 9091-9096, 9191-9196, 19092, 9090) and must not run
together.

**Container-naming near-miss, worth flagging explicitly**: the first
draft of `docker-compose.deploy.yml` reused `container_name:
claimfraud-broker`/`claimfraud-prometheus` — identical to
`docker-compose.yml`'s. Running `docker compose -f
docker-compose.deploy.yml up` against those names silently *recreated*
the existing dev-stack broker container under the new compose project,
with different command args and a different port mapping, rather than
erroring or creating an independent container. Caught immediately (the
recreated broker had the wrong listener config), undone by tearing the
deploy stack down and re-running `docker compose up -d` on the original
file to restore it — no data was actually at risk (Redpanda here holds
only ephemeral test traffic, not a persistent store), but it could have
silently broken the dev stack for a session that didn't notice. Fixed
by prefixing every container name in the deploy file with
`claimfraud-deploy-`. **Lesson for any future compose file in this
repo: never reuse a `container_name` across compose files unless the
intent is explicitly to adopt that existing container** — compose
treats a name collision as "manage this container," not as a conflict
error, with no file-origin check.

**Kafka broker needed two listeners, not one**: the dev broker
advertises a single address (`localhost:19092`) because every client is
host-side. In the deploy stack, `orchestration-service` (a container)
and anything on the host both need to reach the broker, and Kafka
clients reconnect using whatever address the broker hands back in its
own metadata response — one advertised address can't serve both
audiences. Fixed with Redpanda's multi-listener flags: `PLAINTEXT` on
`9092` advertised as `broker:9092` (container-to-container), `OUTSIDE`
on `19092` advertised as `localhost:19092` (host access), both declared
in `--kafka-addr`/`--advertise-kafka-addr`.

**`depends_on` without healthchecks reproduces the same WarmUp race
TC-11/TC-12 already documented for host-native startup** — it only
waits for "container started," not "gRPC server actually accepting
connections," so a claim submitted immediately after `docker compose
up` can still hit `DeadlineExceeded` while the JVM services are mid-boot
(Spring Boot takes noticeably longer to become ready than the Go
binaries). Not fixed here — same accepted caveat as the existing docs,
just reproduced in the containerized path too. A real fix would be
`healthcheck:`/`depends_on: condition: service_healthy` entries, left
for when this moves past demo-grade.

**Known gap, explicitly not solved this pass**: `proto/gen/go` is
gitignored (regenerated locally via `proto/generate.sh`, never
committed — see `monorepo-structure`/that script's own comment). Every
Go service's `Dockerfile` depends on it already existing on disk before
`docker build`/`docker compose build` runs. A fresh `git clone` on a
Hostinger VM would **not** have it — building images on the VM directly
from a clone will fail until this is addressed (commit the generated
code, or install the protoc toolchain in the Go build stage, or build
images locally and ship them to the VM instead of building there).
Deliberately deferred to the actual VM-deployment step, not this one.

**Verified working end-to-end** (local Docker Desktop, 2026-11-16):
`docker compose -f docker-compose.deploy.yml up -d --build` built all 7
images clean and started all 9 containers (7 services + broker +
Prometheus). Once warmed up, `POST /v1/claims` against
`claimfraud-deploy-claims-gateway:8080` returned the full happy-path
result (`status:"scored"`, `fraud_score:0.05`, `model_version:"rules-v1"`,
correctly normalized address) — matching TC-01 exactly, through
container-to-container gRPC the whole way. The DAG's Kafka `publish`
node also succeeded (`"published to kafka"`, topic `claims.realtime`)
confirming the dual-listener broker config. All 7 Prometheus targets
showed `"health":"up"` via their compose service-name addresses. Not
yet deployed to an actual VM — that's the next step.

## 20. Fixed 502s from cold gRPC reconnects after long idle gaps — widened timeout budgets and added client keepalive, deliberately both

Surfaced by the #19 deploy stack itself sitting idle for hours between
manual test hits: `POST /v1/claims` returned `502 Bad Gateway` in
~300ms. orchestration-service's logs showed every enrichment node
(`claimant_id_hash`, `address_normalize`, `policy_lookup`) failing with
`DeadlineExceeded ... while waiting for connections to become ready` —
not one dependency down, all of them, intermittently, while
`docker stats` showed the host otherwise idle.

**Root cause**: `grpc.NewClient` dials lazily and grpc-go's default
channel `idle_timeout` (30 min) drops a quiet connection back to `IDLE`.
#17's `telemetry.WarmUp` only pays the reconnect cost once at startup —
it doesn't re-fire after a channel re-idles later. `claim.fnol.yaml`'s
enrichment nodes budgeted **40ms per attempt** (80ms with their one
retry) — nowhere near enough for a fresh TCP/HTTP2 handshake over Docker
Desktop/WSL2 networking, so the first request after a multi-hour gap
reliably blew its budget, hit `fail_fast`, and claims-gateway turned
that into a 502 (`internal/handler/claims.go`'s `orchestration call
failed` branch).

**Shipped two fixes together, not either/or**: widening the timeout
budget alone would make a cold reconnect *survivable* but wouldn't
reduce how often a live request actually pays for one; keepalive alone
narrows that window but can't be relied on as the whole fix, because
production will always eventually force a fresh dial regardless of
keepalive — pod restarts, rolling deploys, a cloud LB/NAT silently
evicting an idle TCP session it was tracking. A timeout sized only for
"channel is already warm" fails on every one of those in prod, not just
once every 19 hours like this local demo.

1. **Widened the budget** (`claim.fnol.yaml` v4→v5): the three
   enrichment nodes and `model_score` went from 40ms/80ms to **150ms**
   per attempt — claims-gateway's own orchestration-call deadline is 5s,
   so there's ample room. `config/tenants.yaml`'s `dag_versions.auto`
   bumped 4→5 to match (a log-only version-mismatch warning otherwise,
   not a hard failure, but worth keeping in sync).
2. **Added client-side keepalive** (`pkg/telemetry/keepalive.go`,
   `KeepaliveDialOption()`) to all 7 outbound gRPC clients (5 in
   orchestration-service, 2 in claims-gateway) — `PermitWithoutStream:
   true` so pings fire during the long idle gaps this targets, not just
   mid-stream. This converts dead-connection discovery from "the next
   live request finds out the hard way" to "a background ping finds out
   and the channel is already reconnecting by the time a request
   arrives" — specifically targets the half-open-TCP/middlebox-eviction
   failure mode, which widening the timeout alone does nothing to
   prevent.

**Self-inflicted near-miss worth flagging**: the first keepalive
interval picked was 20s. grpc-go's (and grpc-java's) default server-side
keepalive *enforcement policy* rejects client pings more frequent than
5 minutes — the actual result was every outbound connection getting
killed with `GOAWAY ENHANCE_YOUR_CALM / too_many_pings` shortly after
each request, i.e. strictly worse than no keepalive (forced reconnect
churn instead of a stable warm connection). Caught immediately via
container logs after redeploying; fixed by raising `Time` to 10 minutes
— comfortably above the 5-minute server-side floor, still well under
the 30-minute idle_timeout it's meant to pre-empt. **Lesson: a
client-side keepalive interval must be picked relative to the peer's
enforcement policy, not just the idle_timeout it's trying to beat** —
the two defaults aren't related to each other and an interval safe
against one can get you punished by the other.

**Verified end-to-end** (local Docker Desktop, 2026-11-19): rebuilt and
restarted `orchestration-service`, `claims-gateway`, and
`tenant-config-svc`; `go build`/`go test` clean across
`services/orchestration-service`, `services/claims-gateway`,
`pkg/telemetry`. Post-fix `POST /v1/claims` returned `200` in 30-150ms
with no `DeadlineExceeded`, no version-mismatch warning, and no GoAway
errors in the logs.

**Not attempted this pass**: a real topology-aware fix (e.g. the
enrichment "group" still runs sequentially per node, not actually
concurrently — see `executor.go`'s own comment on that simplification)
would shrink the worst-case latency further; out of scope for a
connection-health fix. mTLS, mentioned in #18's open list, remains
open.

## 21. Terraform on OCI for the actual VM deploy, with a registry-image pipeline instead of building on the VM

Continues #19's "containerize, then deploy to a VM" plan — picks the
actual cloud (Oracle Cloud now, AWS later, per the user's own framing)
and the provisioning tool (Terraform, specifically for cheap, complete
teardown via `terraform destroy`, which matters for a demo-grade
project nobody wants silently billing after a session ends).

**OCI over the originally-planned Hostinger VM**: OCI's Always Free tier
includes an Ampere A1 (ARM) shape up to 4 OCPU/24GB — enough to run all
7 services + broker + Prometheus on one box, for free, which beats a
paid Hostinger VM for a demo-grade deployment with no real traffic.
Tradeoff: Ampere A1 is **ARM (aarch64)**, not x86_64 — every image has
to be built for `linux/arm64` specifically (see
`scripts/build-and-push-oci.sh`'s `docker buildx build --platform
linux/arm64`), not just built normally on an x86 dev machine. All of
this repo's base images (`golang`, `eclipse-temurin:25-*`,
`gcr.io/distroless/static-debian12`, Redpanda, Prometheus) already
publish `arm64` manifests, so this cost is real but small — no base
image had to change.

**Terraform scope is deliberately narrow: networking + the VM only**
(`infra/terraform/oci/`) — a VCN, one public subnet, an NSG opening
only `22` (SSH, restricted via `ssh_allowed_cidr`), `8080` (REST), and
`9090` (Prometheus UI; the gRPC/per-service-metrics ports stay
container-internal, reachable only via an SSH tunnel), and one
`VM.Standard.A1.Flex` instance. Not provisioning OKE/Kubernetes, a load
balancer, or multiple instances — there's one service surface and no
real traffic, so a single VM running `docker-compose.deploy.yml`
(already built, #19) is the right amount of infrastructure, same
"smaller, verifiable, one layer at a time" reasoning as every other
thin-slice decision in this file.

**Solves #19's "known gap, explicitly not solved that pass" (`proto/gen/go`
doesn't exist on a fresh clone, so `docker compose build` on the VM
fails) by not building on the VM at all.** `scripts/build-and-push-oci.sh`
builds all 7 images locally — where `proto/gen/go` already exists,
generated via `proto/generate.sh` — and pushes them to OCIR
(`<region>.ocir.io/<namespace>/claimfraud/<service>`). The VM only ever
runs `docker compose -f docker-compose.deploy.yml -f
docker-compose.deploy.images.yml pull && up -d` — the new
`.images.yml` override swaps every service's `build:` for a pre-built
`image:`, so `docker compose` never tries to build anything there.
Considered instead: installing a protoc toolchain in the Go
`Dockerfile`s' build stage so the VM really could `git clone && docker
compose build` — rejected as more moving parts (protoc install, plugin
versions) duplicated into every Go Dockerfile for a problem the
registry pipeline sidesteps entirely.

**The VM never clones this repo at all, and needs no repo credentials**
(public or private). `infra/terraform/oci/compute.tf` reads
`docker-compose.deploy.yml`, the new `docker-compose.deploy.images.yml`,
and the handful of `config/` files the deploy stack needs, straight off
local disk at `terraform apply` time, and ships them to the instance as
base64-encoded cloud-init `write_files` entries. This also means
"what's running" is pinned to exactly what was applied, not whatever
`HEAD` happened to be at boot — a property git-clone-on-boot wouldn't
have given for free.

**Auth token embedded in cloud-init user_data, in plaintext — an
accepted demo-grade shortcut, not an oversight.** OCIR doesn't accept
the Terraform provider's own API-signing-key auth for `docker login`;
it needs a separate auth token (`var.ocir_auth_token`, marked
`sensitive` in Terraform's own output-redaction sense, which does
**not** stop it from being readable in plaintext from the instance's
own metadata service). The correct production fix is instance-principal
auth (a dynamic group + IAM policy granting the VM itself pull rights,
no embedded secret at all) — not done here, flagged explicitly in
`infra/terraform/oci/README.md`'s "what's deliberately not here"
section, same convention as every other accepted shortcut in this file.

**No remote Terraform state, no CI.** `terraform.tfstate` is local and
gitignored — correct for a solo project where only one machine ever
runs `apply`; would need an S3/OCI-Object-Storage backend the moment
that's no longer true. `scripts/build-and-push-oci.sh` is run by hand;
a GitHub Actions workflow invoking it on every push to `main` is the
obvious next step, not done here.

**This code will not port to AWS later, even though the workflow will.**
OCI's resource model (compartments, VCNs, NSGs) and AWS's (VPC, EC2
security groups, IAM roles) are different enough that
`infra/terraform/aws/` will be a rewrite of the resource blocks, not a
provider swap on the same file. What does carry over: the
registry-image-pull pattern (ECR instead of OCIR), the cloud-init
`write_files` approach, and the `plan`/`apply`/`destroy` workflow
itself — worth being explicit about so "Terraform now, AWS later" isn't
read as "same code, new provider."

**Verified locally, 2026-11-23 — `fmt`/`init`/`validate` pass, and all 7
images actually cross-build for `linux/arm64`.** This closes the original
"written but unexecuted" gap for everything that doesn't need a live OCI
account: `terraform fmt -check`, `init`, and `validate` all succeed
(this machine's own broken IPv6 path to `registry.terraform.io` was the
only blocker — a local-network issue, not a `.tf` problem; fixed with a
hosts-file IPv4 pin, see RUNBOOK.md's Troubleshooting entry). Two real
bugs turned up and were fixed in the process, both exactly the kind of
thing this verification pass exists to catch before `terraform apply`
or a push:
- `scripts/build-and-push-oci.sh:25` had a literal apostrophe inside a
  `${VAR:?message}` parameter-expansion message ("...for your region's
  key") — confirmed via isolated repro that this breaks bash's parser
  even though the whole construct is inside double quotes (the `:?word`
  text gets its own internal quote handling). The script would have
  failed on its very first invocation. Fixed by rephrasing the message
  to avoid the apostrophe.
- All 3 Java Dockerfiles' Maven-archive extraction (`tar -xzf`) failed
  with `Function not implemented` specifically when cross-building via
  `docker buildx --platform linux/arm64` through QEMU emulation on this
  amd64 dev machine — a QEMU user-mode syscall-coverage gap (confirmed
  reproducible even after refreshing binfmt handlers and recreating the
  builder from scratch), not a Dockerfile logic error, and not something
  that would happen on the real Ampere VM's native arm64 hardware.
  Fixed by installing `libarchive-tools` and extracting with `bsdtar`
  instead of GNU `tar` in all three Java Dockerfiles — surgical, keeps
  this entry's build-locally-push-to-OCIR architecture completely
  intact. After the fix, all 7 services (4 Go, 3 Java) were actually
  built for `linux/arm64` end-to-end, including the Java build stage's
  full `mvn package` — which is the real, empirical confirmation that
  `os-maven-plugin`'s dynamic `linux-aarch_64` protoc resolution
  (asserted but untested in this entry's original text) genuinely works
  under real cross-compilation, not just in theory.

**Still not applied against a real OCI account.** `terraform plan`
needs real credentials (tenancy/user OCID, API signing key, compartment,
SSH key, OCIR auth token — see `infra/terraform/oci/README.md`
"Prerequisites") that don't exist yet, and the actual `--push` to OCIR
and `terraform apply` are both still ahead. Everything verifiable
without those now has been verified; treat only the `plan`/`apply`/
real-registry-push path as still unexecuted.

## 22. Added AWS as a third deploy target, cross-cloud resource tagging, and a teardown script

Continues #21 directly. OCI's Ampere A1 capacity turned out to be
persistently, not just momentarily, oversubscribed — two manual retries
(daytime, overnight) plus an 11-attempt automated loop rotating all 3
ADs all failed with the same `Out of host capacity` error. Rather than
keep polling, added two more deploy targets: Azure (`infra/terraform/
azure/`, `terraform plan` verified clean against a live account, `apply`
not yet run — the account became inaccessible before that step) and AWS
(`infra/terraform/aws/`, fully verified below).

**AWS targets `t3.medium` (x86_64, 4GB), deliberately not the free-tier
`t2.micro`/`t3.micro` (1GB)** — that size carries the same
resource-starvation risk already rejected for GCP's e2-micro at the very
start of this effort (the stack measures ~800MB+ RSS at idle alone,
before OS/Docker overhead). This is a real, paid instance; the accepted
tradeoff is verify-then-tear-down-immediately, not an attempt to find a
free tier that fits.

**A dedicated, narrowly-scoped IAM user, not the account's default
credentials.** Automated infrastructure that creates and destroys real
paid resources got its own IAM user (`AmazonEC2FullAccess` only — no
IAM permissions, since nothing here creates roles/instance profiles),
authenticated via a named AWS CLI profile (`aws configure set ... --profile
claimfraud`) rather than overwriting or reusing the account's existing
default-profile credentials. The access key was generated and written
into that profile entirely via CLI (`aws iam create-access-key` piped
straight into `aws configure set`) — the secret never appeared in chat,
same practice as every other credential handled this effort.

**Validated AWS-specific Terraform correctness in a dedicated research
pass before writing any `.tf`** — real money on the line, no live
hands-on AWS testing yet unlike OCI/Azure. Confirmed and then verified
live against the account: the Canonical Ubuntu 22.04 x86_64 AMI
lookup (`owners = ["099720109477"]`, the standard jammy HVM/SSD name
filter, plus a `root-device-type = "ebs"` filter since `t3.medium` is
EBS-only); that AWS has no OCI/Azure-style "pass an SSH key directly as
instance metadata" mechanism and needs an explicit `aws_key_pair`
resource; that `aws_instance.user_data` takes **plain text** (Terraform/
AWS base64-encodes it internally — wrapping it in `base64encode()` the
way OCI's `metadata.user_data`/Azure's `custom_data` both require would
double-encode and break cloud-init parsing); that a custom
`aws_security_group` with no `egress` block has **zero** outbound rules,
not an implicit allow-all (AWS doesn't carry the default-VPC-SG's
allow-all-egress behavior over to a security group you create yourself
— needs an explicit allow-all egress rule, same reasoning as OCI's NSG,
unlike Azure's NSG which does ship a built-in default); and that
`associate_public_ip_address = true` directly on the instance is
sufficient, no `aws_eip` needed (an EIP left behind would itself cost
money if forgotten, a real risk for something meant to be short-lived).
`terraform plan` against the live account confirmed all of this: 8
resources to add, 0 errors, correct AMI resolution, before any `apply`.

**Cross-cloud resource tagging — `common_tags` (`../shared.tfvars`),
applied through three different mechanisms, not three different
schemas.** AWS v5.x supports a provider-level `default_tags` block that
auto-tags every taggable resource with zero per-resource repetition —
the cleanest of the three. Azure's `azurerm` provider has no equivalent;
`tags = var.common_tags` was added explicitly to each resource that
supports it (`azurerm_subnet` and `azurerm_network_security_rule` don't
support tags at all — Azure subnets aren't independently taggable).
OCI's resources use `freeform_tags = var.common_tags` the same way
(its own NSG security rules have the same no-tags-on-sub-resources
limitation). Tags update in place on all three providers (confirmed via
a live `terraform plan` on OCI's 9 already-created resources: 5 to
change, 0 to destroy, no forced replacement) — a meaningfully different
risk profile than renaming a resource itself, which is why this was
safe to apply retroactively to OCI's live resources while
`display_name_prefix` (which drives actual resource names) was not.

**`claimfraud` → `claimsfraud` naming correction, scoped to new things
only, not a full rename.** Caught mid-session that "claimsfraud" is the
more correct name. Renaming `display_name_prefix`'s shared default would
force Terraform to destroy and recreate OCI's 9 already-live resources
(re-rolling the Ampere A1 capacity lottery for no real benefit) and
require re-pushing all 7 images under a new OCIR path three other files
already depend on — correctly judged not worth it. Landed on: the new
`common_tags.Project` value uses "claimsfraud-platform" (tags update in
place, no disruption either way), and `infra/terraform/aws/`
specifically — nothing created there yet — overrides
`display_name_prefix` to `"claimsfraud"` in its own `terraform.tfvars`
rather than changing the shared default, so its resources are named
`claimsfraud-*` from the start without touching OCI/Azure's naming.

**`scripts/teardown-cloud.sh <oci|azure|aws>`** — a thin wrapper around
`terraform destroy -var-file=../shared.tfvars -var-file=terraform.tfvars`
for whichever cloud, so the two flags don't need retyping each time,
especially for AWS's paid instance which is meant to be torn down
promptly after verification. Deliberately does **not** pass
`-auto-approve` — destroy is the one operation in this whole effort
where skipping the interactive "type yes" confirmation would be a real
safety regression, not just friction.

**`PLATFORM` env var + `image_tag` template variable, for the second
(x86_64) image build.** `scripts/build-and-push-oci.sh`'s hardcoded
`--platform linux/arm64` became `PLATFORM="${PLATFORM:-linux/arm64}"`
(zero behavior change for existing OCI/Azure use). All three clouds'
`cloud-init.yaml.tftpl` (still byte-for-byte identical to each other)
gained an `IMAGE_TAG=${image_tag}` on the existing `docker compose
pull`/`up -d` lines; OCI's and Azure's `compute.tf` pass `"latest"`
(arm64, no behavior change), AWS's passes `"latest-amd64"` — a distinct
tag on the *same* OCIR registry, not a second registry or pipeline.

**Verified end-to-end for AWS** (2026-11-27): `terraform fmt`/`init`/
`validate` clean; live `terraform plan` against the real account (scoped
`claimsfraud-terraform` IAM user, confirmed via `aws sts
get-caller-identity`) resolved a real AMI ID and planned exactly 8
resources with 0 errors. **Not yet done**: the x86_64 image build/push,
`terraform apply`, and the teardown-script exercise itself — those
remain the next concrete steps, same "verified building block, not yet
the full round trip" status Azure was left at.

## 23. Full AWS round trip — found and fixed two real bugs the OCI/Azure plans never surfaced

Continues #22 directly: image build/push succeeded (all 7, `:latest-amd64`,
verified live on OCIR), then `terraform apply` — which is where two real
bugs surfaced, neither caught by `plan`/`validate` on any of the three
clouds, because both only manifest once an instance actually boots.

**Bug 1 — `t3.medium` isn't supported in every AZ.** The first `apply`
failed at `aws_instance.this` with `Unsupported: ... t3.medium ... not
supported in your requested Availability Zone (us-east-1e)` — AWS had
auto-placed the subnet in `us-east-1e`, one of `us-east-1`'s more
restricted AZs, since `aws_subnet.public` left `availability_zone`
unset. Fixed by adding an explicit `availability_zone` variable
(default `us-east-1a`, one of the AZs the error itself listed as
working) rather than leaving placement to chance. Re-running `apply`
correctly showed only the subnet and its route table association as
needing replacement (2 destroyed, 3 added) — the VPC, IGW, route table,
security group, and key pair were untouched, confirming the fix was as
narrow as intended.

**Bug 2 — real, and present in all three clouds' `compute.tf`, not AWS-
specific: `locals.deploy_files` stored path strings, not file
contents.** The instance came up, but `docker compose` failed on
*every* file with `yaml: construct errors: ... cannot construct !!str`.
Live debugging on the running instance (`docker compose ... config
--quiet` failed identically with zero file args beyond a single `-f`,
ruling out OCIR/credentials entirely) traced it to `cat
/opt/claimfraud/docker-compose.deploy.yml` literally containing the
text `./../../../docker-compose.deploy.yml` — the `deploy_files` map in
`compute.tf` interpolated each value as `"${local.repo_root}/<name>"` (a
path string) instead of wrapping it in Terraform's `file(...)` function,
so `cloud-init.yaml.tftpl`'s `base64encode(content)` was faithfully
encoding the *path*, not the file it pointed to. All 5 shipped files
(`docker-compose.deploy.yml`, `.deploy.images.yml`, `tenants.yaml`,
`prometheus.docker.yml`, `claim.fnol.yaml`) had the identical symptom —
confirmed directly on the instance before writing the fix. **This bug
was dormant in OCI's and Azure's `compute.tf` too**, written the same
way when Azure was built from OCI's pattern and AWS from both — neither
`terraform validate` nor `plan` catches it, since `file()`'s absence is
syntactically valid HCL that only fails once cloud-init actually tries
to use the bogus "content" on a real boot. Fixed in all three
directories' `compute.tf` by wrapping each `deploy_files` value in
`file(...)`.

**Live-patched the already-running instance directly** (`scp` the real
files to `/tmp`, `sudo mv` into place, re-run the two `docker compose`
commands by hand) rather than wait for a destroy/recreate cycle —
`aws_instance.user_data` changes don't force replacement by default
(`user_data_replace_on_change = false`), so simply re-`apply`-ing after
the fix would have updated the stored `user_data` without ever
re-running cloud-init on the live instance. This got a real, same-
session verification without burning a second `t3.medium` boot; a
*fresh* `apply` from a clean account would pick up the fix correctly
through cloud-init as normal.

**Verified fully end-to-end on AWS** (2026-11-28): first claim after
`up -d` came back `"status":"degraded"` (`policy_lookup` and the Kafka
`publish` node both hit `DeadlineExceeded` — the same documented
cold-connection/JVM-warm-up race as RUNBOOK.md's existing OCI/Azure
entries, not a new issue), a second claim ~20s later came back
`"status":"scored"` in 134ms. Prometheus also confirmed reachable
(`/targets` → 200).

**Not yet done**: `scripts/teardown-cloud.sh aws` itself hasn't been
exercised yet — still the explicit next step, and the instance should
not be left running past that given it's real paid compute.
