# Insurance Fraud-Detection Platform

Multi-tenant insurance claims fraud/risk-scoring platform. See [DESIGN.md](DESIGN.md)
for the full architecture, [DECISIONS.md](DECISIONS.md) for why it's being
built in this order and what's been simplified at each layer,
[docs/dag-request-flow.md](docs/dag-request-flow.md) for a file-by-file trace
of what calls what for a single claim submission, and
**[RUNBOOK.md](RUNBOOK.md) for how to build, run, test, and troubleshoot it**.

## Status

Walking skeleton (ClaimsGateway → Orchestration → Address Normalization →
stub Model Service) is done. Now on **layer 2a**: a real Tenant Config
Service (thin slice — local YAML, no Postgres/Kafka yet) and a config-driven
DAG execution engine in Orchestration, replacing both the hardcoded tenant
and the hardcoded DAG. See [DECISIONS.md](DECISIONS.md) for what's still
deferred (remaining enrichment services, resilience/observability, batch
flow, and the rest of layer 2's own scope: Postgres, transactional outbox,
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
│   ├── address-normalization-svc/   # Java 25 / Spring Boot / Maven — enrichment
│   └── model-service/               # stub fraud-scoring service
├── frontend/                        # React
└── infra/docker/                    # per-service Dockerfiles
```

## Prerequisites

- Go 1.25+ (the local SDK must actually be ≥1.25, not just auto-toolchained)
- Java 25 (Temurin) + Maven 3.9+
- Docker (for Kafka/Postgres via docker-compose, once wired in)

## Running, testing, troubleshooting

See **[RUNBOOK.md](RUNBOOK.md)** — build/regenerate-proto steps, the run
order for all five services, a smoke-test curl, how to run the Go and Java
test suites, and fixes for the gotchas already hit while building this
(gopls/toolchain mismatch, Spring Boot gRPC daemon-thread exit).
