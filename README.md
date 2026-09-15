# Insurance Fraud-Detection Platform

Multi-tenant insurance claims fraud/risk-scoring platform. See [DESIGN.md](DESIGN.md)
for the full architecture, [DECISIONS.md](DECISIONS.md) for why it's being
built in this order and what's been simplified at each layer,
[docs/dag-request-flow.md](docs/dag-request-flow.md) for a file-by-file trace
of what calls what for a single claim submission,
[docs/manual-test-cases.md](docs/manual-test-cases.md) for reusable
REST/gRPC test cases (request/response/expected-result, re-runnable via a
console tool or raw curl), and
**[RUNBOOK.md](RUNBOOK.md) for how to build, run, test, and troubleshoot it**.

## Status

Walking skeleton done. Layer 2a (Tenant Config Service + config-driven DAG
execution engine, replacing the hardcoded tenant and hardcoded DAG) done.
Done with **all three Java enrichment services** from DESIGN.md §4:
Claimant ID Hashing and Policy Lookup joined Address Normalization in the
DAG's "enrichment" group — both thin slices (static-salt hash, fixture-backed
lookup). The `publish` stage now does a **real Kafka publish** (Redpanda via
`docker-compose.yml`, JSON to `claims.realtime`, `on_failure: skip`),
replacing the log-only stub — see [DECISIONS.md](DECISIONS.md) #15 for why
JSON rather than Protobuf + schema registry. See [DECISIONS.md](DECISIONS.md)
for what's still deferred (resilience/observability, batch flow, real Model
Service, and layer 2's own remaining scope: Postgres, transactional outbox,
Kafka cache invalidation, JSON Schema validation, API-key auth).

## Repo layout

```
claim-fraud-platform/
├── DESIGN.md                       # architecture design doc
├── DECISIONS.md                     # running log of build decisions + rationale
├── RUNBOOK.md                        # how to build, run, test, troubleshoot
├── docker-compose.yml               # local dev infra + services
├── proto/                           # shared .proto contracts (single source of truth)
├── config/                          # tenant + DAG config YAML (shared, not nested in a service)
│   ├── tenants.yaml
│   └── dag/{tenant_id}/{product}/{event_type}.yaml
├── services/
│   ├── claims-gateway/              # Go — REST ingress, REST→gRPC bridge
│   ├── orchestration-service/       # Go — executes the config-driven enrichment DAG
│   ├── tenant-config-svc/           # Go — tenant status + DAG version resolution
│   ├── claimant-id-hashing-svc/     # Java 25 / Spring Boot / Maven — enrichment
│   ├── address-normalization-svc/   # Java 25 / Spring Boot / Maven — enrichment
│   ├── policy-lookup-svc/           # Java 25 / Spring Boot / Maven — enrichment
│   └── model-service/               # stub fraud-scoring service
├── frontend/                        # React
└── infra/docker/                    # per-service Dockerfiles
```

## Prerequisites

- Go 1.25+ (the local SDK must actually be ≥1.25, not just auto-toolchained)
- Java 25 (Temurin) + Maven 3.9+
- Docker (for the Kafka broker via `docker-compose.yml`; Postgres not wired
  in yet)

## Running, testing, troubleshooting

See **[RUNBOOK.md](RUNBOOK.md)** — build/regenerate-proto steps, the run
order for all seven services, a smoke-test curl, how to run the Go and Java
test suites, and fixes for the gotchas already hit while building this
(gopls/toolchain mismatch, Spring Boot gRPC daemon-thread exit).
