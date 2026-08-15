# Insurance Fraud-Detection Platform — Design Document

## 1. Overview

A multi-tenant insurance claims fraud/risk-scoring platform, tenants are
insurance carriers (e.g. `acme_insurance`). Two flows:

- **Real-time flow:** a claim event is submitted, enriched via a
  tenant-configurable DAG of parallel service calls, scored by a model, and
  published to Kafka / the Data Lake.
- **Batch flow:** historical claims are periodically reprocessed
  (decrypt → normalize → hash → cleanse → re-encrypt) and republished.

## 2. Tech Stack

| Layer | Technology |
|---|---|
| Frontend | React |
| Backend services | Mix of Go and Java (Spring Boot + Spring Cloud) |
| Internal comms | gRPC (protobuf) |
| Edge/API | REST, translated to gRPC at the gateway |
| Messaging | Kafka (Protobuf + schema registry) |
| Config/discovery | Spring Cloud Config, Eureka/Consul |
| Resilience | Resilience4j (Java), equivalent circuit-breaker lib (Go) |
| Tracing | OpenTelemetry → Jaeger/Tempo |
| Metrics | Prometheus + Grafana |
| Logging | Structured JSON → Loki/ELK |
| Storage | Postgres (tenant/config data), S3 (Data Lake, Parquet) |
| Secrets | Vault or AWS Secrets Manager |
| Encryption | Per-tenant KMS keys, field-level encryption for PII |

## 3. Requirements & SLAs (to finalize with concrete numbers before build)

- End-to-end real-time latency budget (e.g. sub-300ms for a fraud score at
  claim submission)
- Expected throughput per tenant and in aggregate
- Event types: `claim.fnol` (first notice of loss), `claim.update`,
  `claim.payment_request`
- Products: auto, home, health (extensible)

## 4. Real-Time Flow — Services

1. **ClaimsGateway (Go)** — REST ingress, REST→gRPC bridge, tenant
   resolution, rate limiting, auth/scopes.
2. **Orchestration Service (Go)** — executes a per-tenant/product/eventType
   DAG of enrichment calls with parallel groups, per-node timeout/retry/
   failure policy.
3. **Claimant ID Hashing (Java, Spring Boot)** — enrichment, hashes PII
   identifiers.
4. **Address Normalization (Java, Spring Boot)** — enrichment.
5. **Policy Lookup (Java, Spring Boot)** — enrichment, resolves policy
   number to coverage details, prior claims.
6. **Model Service** — real-time fraud/risk inference on the assembled
   feature map; outputs a score + optional feature-level explanations.
7. **Kafka → Consumer → Data Lake** — publish scored events, sink to
   storage.

## 5. Batch Flow — Services

- **Batch Transform Service (Java, Spring Batch)** — consumes the
  historical Kafka topic, runs decrypt → address normalize (reuses the
  real-time Address Normalization service via gRPC) → claimant ID hash
  (reuses the real-time service) → cleanse → re-encrypt, republishes to
  Kafka and the Data Lake. Scheduled (cron / Kubernetes CronJob), not a
  continuous consumer.

## 6. Multi-Tenancy

- **Tenants = insurance carriers.**
- **Isolation model: Bridge (hybrid).**
  - Shared compute for ClaimsGateway, Orchestration Service, and all
    stateless enrichment services — routed and behaviorally customized
    per tenant via the DAG config.
  - Isolated storage/keys for anything sensitive: per-tenant KMS keys,
    field-level PII encryption, Data Lake partitioned/prefixed by
    `tenant_id`.
  - Rejected alternatives: full **silo** (dedicated infra per carrier —
    too much repetition, doesn't teach multi-tenancy patterns) and pure
    **pool** (shared everything with just a `tenant_id` column —
    under-represents real data-segregation requirements in insurance).

### 6.1 Tenant-Aware DAG Config

Stored as versioned YAML (Git + JSON Schema validation on merge), served
via a Config Service, hot-reloadable via Kafka event rather than polling.

```yaml
tenant_id: acme_insurance
product: auto
event_type: claim.fnol
version: 3
timeout_ms: 250

nodes:
  - id: claimant_id_hash
    service: claimant-id-hashing-svc
    group: enrichment
    timeout_ms: 40
    on_failure: fail_fast

  - id: address_normalize
    service: address-normalization-svc
    group: enrichment
    timeout_ms: 40
    on_failure: skip

  - id: policy_lookup
    service: policy-lookup-svc
    group: enrichment
    timeout_ms: 40
    on_failure: degrade

  - id: siu_referral            # carrier-specific extra step
    service: acme-siu-svc
    group: enrichment
    timeout_ms: 60
    on_failure: fail_fast
    enabled_for: [acme_insurance]

  - id: model_score
    service: model-service
    depends_on: [enrichment]
    timeout_ms: 80

  - id: publish
    service: kafka-publisher
    depends_on: [model_score]
```

- `on_failure` per node: `fail_fast` | `skip` | `degrade`.
- `enabled_for` gates tenant-specific nodes without forking the whole DAG.
- Orchestration Service loads DAGs **lazily** per `(tenant_id, product,
  event_type)`, caches the compiled/topo-sorted DAG in memory, keyed by
  `tenant_id:product:event_type:version`.
- Cache invalidation via `dag-config.updated` Kafka event (same pattern
  as Tenant Config Service, below) — evict on update, re-fetch on next
  request. No mid-flight version switching.
- Fallback on Config Service outage: serve stale cached DAG if one
  exists; `fail_fast` only if no cached version is available at all.

## 7. Tenant Config Service

Single source of truth for tenant-specific data needed on the hot path.

**Data model (per tenant):**
```yaml
tenant_id: acme_insurance
display_name: "Acme Insurance Co"
status: active                    # active | suspended | onboarding
api_keys:
  - key_hash: "sha256:..."
    scopes: [claims.write, claims.read]
rate_limit:
  requests_per_second: 200
  burst: 500
encryption:
  kms_key_id: "arn:aws:kms:...:key/acme-insurance-key"
dag_config_ref:
  product_overrides:
    auto: v3
    home: v2
compliance:
  data_residency: us-east-1
  audit_log_required: true
```

**gRPC API:**
- `ResolveTenant(api_key_hash) → TenantContext`
- `GetEncryptionKey(tenant_id) → kms_key_ref`
- `GetDagVersion(tenant_id, product) → version`
- `GetRateLimit(tenant_id) → quota`

**Caching:** each caller (ClaimsGateway, Orchestration, enrichment
services) keeps a local in-memory cache (TTL ~30-60s) of tenant data.

**Invalidation:** transactional outbox pattern — tenant writes and an
outbox row committed in the same Postgres transaction; a poller/CDC
process (or Debezium) publishes to `tenant-config-events` (partitioned by
`tenant_id`). Event payload carries only `tenant_id` + `changed_fields` +
`version`, never the full object — subscribers evict the cache entry and
re-fetch on next read, rather than merging partial data.

## 8. ClaimsGateway Request Lifecycle

1. TLS termination / request parsing.
2. **Tenant resolution** — API key/JWT → `ResolveTenant` (cached) →
   `401` on failure. Nothing downstream runs before this succeeds.
3. Tenant status check (`403` if suspended/onboarding).
4. **Rate limiting** — token bucket keyed by `tenant_id`, quota from
   `TenantContext` (`429` if exceeded).
5. Scope/permission check (`403` if missing scope).
6. Request/schema validation.
7. REST → gRPC translation.
8. **Tenant context injection** into outgoing gRPC metadata
   (`x-tenant-id`), read by every downstream service via interceptor —
   never passed in the message payload.
9. Response translation back to REST/JSON.

Correlation ID generated at step 1 (or passed through), propagated the
same way as `tenant_id`, ties OpenTelemetry traces together across Go and
Java services.

## 9. Model Service

- Java + ONNX Runtime (or a Go client to a dedicated inference runtime
  like TorchServe/Triton).
- Input: the feature map assembled by Orchestration from all enrichment
  nodes, passed as a single gRPC request.
- Must handle missing/degraded features gracefully (trained-in defaults,
  not nulls).
- Model artifacts versioned separately (S3), `model_version` resolved
  per tenant/product the same way as DAG versions — hot-swappable
  without redeploy.
- Output: fraud score (0–1) + optional feature-level explanations
  (useful for adverse-action/explainability requirements in insurance).

## 10. Kafka & Data Design

- `claims.realtime` — scored claim events, partitioned by `tenant_id`
  (prefer partitioning over per-tenant topics to avoid topic sprawl).
- `claims.historical` — raw historical claims feeding Batch Transform.
- `tenant-config-events`, `dag-config.updated` — cache-invalidation
  topics.
- Protobuf + schema registry (Confluent or AWS Glue) for backward
  compatibility as schemas evolve.
- **Data Lake:** S3, Parquet, partitioned by
  `tenant_id / product / event_date`.

## 11. Observability

- **Tracing:** OpenTelemetry in every service, correlation ID as trace
  root, exported to Jaeger/Tempo.
- **Metrics:** RED (rate, errors, duration) per service via Prometheus,
  tagged with `tenant_id` for per-carrier slicing.
- **Logging:** structured JSON, always including `correlation_id` and
  `tenant_id`, central store (Loki/ELK). Never log raw claim PII.

## 12. Security & Compliance

- mTLS between all internal gRPC services.
- Per-tenant KMS keys, field-level encryption for PII before it reaches
  Kafka or the Data Lake.
- Immutable, append-only audit log for claimant-data access, separate
  from application logs (different retention/access-control needs).
- Secrets via Vault/AWS Secrets Manager — never in config files or env
  vars directly.

## 13. Open Items Before/During Implementation

- Finalize concrete SLA numbers (latency, throughput) per tenant tier.
- Decide DAG Config Store: separate service vs. a table within Tenant
  Config Service (leaning toward colocating for simplicity).
- Decide service mesh (Istio/Linkerd) vs. manual mTLS/cert rotation.
- Define onboarding workflow for a new carrier (config + key
  provisioning, ideally no code deploy).
