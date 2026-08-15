# Insurance Fraud-Detection Platform

Multi-tenant insurance claims fraud/risk-scoring platform. See [DESIGN.md](DESIGN.md)
for the full architecture.

## Status

Building a **walking skeleton** first: a thin, real request flowing end-to-end
through ClaimsGateway → Orchestration Service → Address Normalization →
(stub) Model Service → (stub) Kafka publish, before layering in the rest
(Tenant Config Service, real DAG config loading, remaining enrichment
services, resilience/observability, batch flow).

## Repo layout

```
claim-fraud-platform/
├── DESIGN.md                       # architecture design doc
├── docker-compose.yml               # local dev infra + services
├── proto/                           # shared .proto contracts (single source of truth)
├── services/
│   ├── claims-gateway/              # Go — REST ingress, REST→gRPC bridge
│   ├── orchestration-service/       # Go — executes the enrichment DAG
│   ├── address-normalization-svc/   # Java 25 / Spring Boot / Maven — enrichment
│   └── model-service/               # stub fraud-scoring service
├── frontend/                        # React
└── infra/docker/                    # per-service Dockerfiles
```

## Prerequisites

- Go 1.23+
- Java 25 (Temurin) + Maven 3.9+
- Docker (for Kafka/Postgres via docker-compose, once wired in)

## Running the walking skeleton

Four processes, each in its own terminal, **in this order** (each dials
the next downstream address lazily, so a couple seconds' head start is
enough — it doesn't have to be exact):

```sh
# 1. Model Service (Go stub) — gRPC :9093
cd services/model-service && go run ./cmd/model

# 2. Address Normalization (Java 25 / Spring Boot) — gRPC :9092
cd services/address-normalization-svc && mvn spring-boot:run

# 3. Orchestration Service (Go) — gRPC :9091, calls #1 and #2
cd services/orchestration-service && go run ./cmd/orchestration

# 4. ClaimsGateway (Go) — REST :8080, calls #3
cd services/claims-gateway && go run ./cmd/gateway
```

Then submit a claim:

```sh
curl -s -X POST http://localhost:8080/v1/claims \
  -H "Content-Type: application/json" \
  -d '{
    "claim_id": "clm-0001",
    "product": "auto",
    "event_type": "claim.fnol",
    "policy_number": "POL-123456",
    "claimant_name": "Jane Doe",
    "raw_address": "123 Main St, Springfield, IL 62704"
  }'
```

Expected response:

```json
{"claim_id":"clm-0001","correlation_id":"<generated>","status":"scored","normalized_address":"123 MAIN ST, SPRINGFIELD, IL, 62704, US","fraud_score":0.42,"model_version":"stub-v0"}
```

`GET http://localhost:8080/healthz` is also available on the gateway.

Ports and downstream addresses are configurable via env vars
(`MODEL_GRPC_ADDR`, `ADDRESS_NORMALIZATION_ADDR`/`app.grpc.port`,
`ORCHESTRATION_GRPC_ADDR`/`ORCHESTRATION_ADDR`, `GATEWAY_HTTP_ADDR`) — see
each service's `main.go`/`application.yml` for defaults.
