# Runbook

Operational reference: how to build, run, test, and stop this platform.
Kept up to date as new services land — when a new service/test suite is
added, extend this file rather than leaving instructions only in chat
history or a commit message.

For *why* it's built this way, see [DESIGN.md](DESIGN.md) (target
architecture) and [DECISIONS.md](DECISIONS.md) (build decisions +
rationale). For *what calls what*, see
[docs/dag-request-flow.md](docs/dag-request-flow.md). For a reusable,
re-runnable set of REST/gRPC test cases (per-endpoint request/response/
expected-result), see [docs/manual-test-cases.md](docs/manual-test-cases.md).

## Prerequisites

- Go 1.25+ — the local SDK must actually **be** ≥1.25, not just
  auto-toolchained. `go build`/`go run` mask an older local SDK via
  `GOTOOLCHAIN=auto`, but `gopls` doesn't (see Troubleshooting below).
- Java 25 (Temurin) + Maven 3.9+
- Docker (for the Kafka broker via `docker-compose.yml` — see step 4a.
  Postgres isn't wired in yet.)

## 1. Regenerate protobuf stubs

Only needed after editing a `.proto` file under `proto/`:

```sh
cd proto
./generate.sh   # or: bash generate.sh
```

Requires `tools/protoc/bin/protoc.exe` (portable, gitignored — see
`proto/generate.sh`'s header comment for how it got there) and
`protoc-gen-go`/`protoc-gen-go-grpc` on `PATH` (`$(go env GOPATH)/bin`,
which the script adds itself). Java stubs are generated separately, at
Maven build time, by each Java service's own `pom.xml`.

## 2. Build & vet every Go module

Go workspaces (`go.work`) require this **per module** — `go build ./...`
from the repo root fails (`directory prefix . does not contain modules
listed in go.work`), since the repo root itself isn't a workspace member.
Either `cd` into each module or path-prefix from the root:

```sh
cd services/tenant-config-svc      && go build ./... && go vet ./...
cd ../claims-gateway                 && go build ./... && go vet ./...
cd ../orchestration-service           && go build ./... && go vet ./...
cd ../model-service                    && go build ./... && go vet ./...
```

All four should exit with no output.

## 3. Run unit tests

**Go** — currently only `orchestration-service/internal/dag` has real tests
(config/executor logic: `on_failure`, `enabled_for`, staged execution).
Extend this list as more packages get tests:

```sh
cd services/orchestration-service
go test ./... -v
```

**Java** — pure logic in each enrichment service:

```sh
cd services/address-normalization-svc && mvn test
cd ../claimant-id-hashing-svc && mvn test
cd ../policy-lookup-svc && mvn test
```

## 4. Run the full stack

### 4a. Kafka broker (Redpanda, via docker-compose)

Start this **before** Orchestration, or its publish calls just fail-soft
(`on_failure: skip` on the `publish` node — see DECISIONS.md #15):

```sh
docker compose up -d
```

Broker listens on **`localhost:19092`** — deliberately not the
conventional `9092`, which address-normalization-svc's own gRPC server
already owns in this repo (`9091`-`9096` below are all gRPC; `19092` is
the one Kafka port in the mix). `docker compose down` to stop it; no
volumes, so topic data doesn't persist across a `down`.

### 4b. The seven services

Two ways to do this — same end state, pick whichever fits:

**Option A: scripted (`scripts/start-all.sh`)**

```sh
./scripts/start-all.sh          # prompts before killing anything
./scripts/start-all.sh -y       # or --yes, skips the prompt
```

Starts all seven in the order below, each backgrounded with its own log
under `logs/<service-name>.log` (tail with `tail -f logs/*.log`). If any of
the seven ports is already bound (e.g. leftovers from an earlier session),
it lists exactly what's using them (port, service, PID, process name) and
asks for confirmation before killing those processes and continuing.
Doesn't touch the Kafka broker (step 4a) or the test console — start those
separately.

**Option B: manual, one terminal per service**

Gives you a live log/Ctrl+C per service in its own terminal, which the
script's backgrounded/logged-to-file model doesn't. Same order, same
reasoning either way — tenant-config-svc first since both ClaimsGateway and
Orchestration depend on it (each service dials its downstream addresses
lazily, so a couple seconds' head start is enough, it doesn't have to be
exact):

```sh
# 1. Tenant Config Service (Go) — gRPC :9094
cd services/tenant-config-svc && go run ./cmd/tenantconfig

# 2. Model Service (Go, rules-based scoring — not ML) — gRPC :9093
cd services/model-service && go run ./cmd/model

# 3. Claimant ID Hashing (Java 25 / Spring Boot) — gRPC :9095
cd services/claimant-id-hashing-svc && mvn spring-boot:run

# 4. Address Normalization (Java 25 / Spring Boot) — gRPC :9092
cd services/address-normalization-svc && mvn spring-boot:run

# 5. Policy Lookup (Java 25 / Spring Boot) — gRPC :9096
cd services/policy-lookup-svc && mvn spring-boot:run

# 6. Orchestration Service (Go) — gRPC :9091, calls #1-#5 + the Kafka broker
cd services/orchestration-service && go run ./cmd/orchestration

# 7. ClaimsGateway (Go) — REST :8080, calls #1 and #6
cd services/claims-gateway && go run ./cmd/gateway
```

Ports and downstream addresses are configurable via env vars
(`TENANT_CONFIG_ADDR`/`TENANT_CONFIG_GRPC_ADDR`, `MODEL_GRPC_ADDR`,
`ADDRESS_NORMALIZATION_ADDR`/`app.grpc.port`,
`CLAIMANT_ID_HASHING_ADDR`/`app.grpc.port`,
`POLICY_LOOKUP_ADDR`/`app.grpc.port`, `KAFKA_BROKER_ADDR` (default
`localhost:19092`), `ORCHESTRATION_GRPC_ADDR`/`ORCHESTRATION_ADDR`,
`GATEWAY_HTTP_ADDR`, `DAG_CONFIG_DIR`, `TENANTS_FILE`) — see each
service's `main.go`/`application.yml` for defaults.

## 5. Smoke test

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
{"claim_id":"clm-0001","correlation_id":"<generated>","status":"scored","normalized_address":"123 MAIN ST, SPRINGFIELD, IL, 62704, US","fraud_score":0.05,"model_version":"rules-v1"}
```

Orchestration's logs will show `claimant_id_hash`, `address_normalize`, and
`policy_lookup` all running (see [docs/dag-request-flow.md](docs/dag-request-flow.md))
— if any of the three new/existing enrichment services isn't up yet, the DAG
config's `on_failure` policy decides what happens: `claimant_id_hash` is
`fail_fast` (whole request fails), `address_normalize` is `skip`, and
`policy_lookup` is `degrade` (both of the latter two let the request
continue with `status: degraded`). The `publish` node (real Kafka publish
to `claims.realtime` as of DECISIONS.md #15) is `on_failure: skip` too —
if the broker from step 4a isn't up, the log will show `dag node=publish
failed, continuing (on_failure=skip)` and the response comes back
`status: "degraded"` rather than failing outright.

To confirm a scored claim actually reached the broker, either consume
`claims.realtime` directly (`docker exec claimfraud-broker rpk topic
consume claims.realtime --brokers localhost:9092 --offset start`, run
*inside* the container — the broker's advertised listener is
`localhost:19092`, which only resolves correctly from the host, not from
inside its own container) or via the test console's Kafka panel
(`localhost:19092`, `claims.realtime`, Avro decode off — see
[docs/manual-test-cases.md](docs/manual-test-cases.md) TC-10).

`GET http://localhost:8080/healthz` is also available on the gateway.

ClaimsGateway currently always resolves the same hardcoded tenant id
(`acme_insurance` — real API-key-based resolution is deferred, see
[DECISIONS.md](DECISIONS.md) #9), but does so via a real gRPC call to
tenant-config-svc rather than assuming it's active. `config/tenants.yaml`
also seeds a second, `suspended` tenant purely to exercise that gate. There
isn't yet a way to hit it via the REST API without temporarily editing
`hardcodedTenantID` in
[claims.go](services/claims-gateway/internal/handler/claims.go), restarting
ClaimsGateway, confirming the `403`, then reverting — that's how it was
verified when this layer was built (see [DECISIONS.md](DECISIONS.md) #9);
`tenant_id` still isn't a client-supplied field.

## 6. Stopping everything

**If you used Option B** (manual terminals): Ctrl+C each of the 7 terminals.

**If you used Option A** (`scripts/start-all.sh`): the seven processes are
backgrounded, not attached to a terminal — either re-run
`./scripts/start-all.sh`, which detects all seven still listening and
offers to kill them before relaunching (say no if you just want them
stopped), or kill them directly:

```sh
powershell.exe -NoProfile -Command "Get-NetTCPConnection -LocalPort 9091,9092,9093,9094,9095,9096,8080 -State Listen | Select-Object -ExpandProperty OwningProcess -Unique | ForEach-Object { Stop-Process -Id $_ -Force }"
```

Either way, finish with `docker compose down` for the broker from step 4a.
Nothing here restarts itself or holds external state beyond the broker's
own topic data (no DB writes), so there's no other cleanup step — just
don't leave stray `go run`/`mvn spring-boot:run` processes (or the broker
container) running before switching branches or rebuilding.

## Troubleshooting

- **VS Code "Go to Definition" doesn't work / gopls errors like `go.work
  requires go >= X.Y.Z (running go A.B.C)`:** your locally *installed* Go
  SDK is older than what `go.work` declares. `go build`/`go run` mask this
  via `GOTOOLCHAIN=auto` (auto-downloads a matching toolchain per
  invocation), but `gopls` does not. Fix: actually upgrade the local SDK
  (don't just rely on auto-toolchain), then restart gopls / reload the VS
  Code window. Check the real local version with `go version` run *outside*
  any directory containing a go.work/go.mod file, to bypass the auto-switch.

- **A Java gRPC service (e.g. address-normalization-svc,
  claimant-id-hashing-svc, policy-lookup-svc) starts, logs "Started
  Application," then the process exits immediately:** a manually-started
  gRPC server in a non-web Spring Boot app needs a non-daemon thread to keep
  the JVM alive — see
  [GrpcServerLifecycle.java](services/address-normalization-svc/src/main/java/io/redcell/addressnorm/grpc/GrpcServerLifecycle.java)
  (each of the three Java services has its own copy of this same class) for
  how this is handled; apply the same pattern to any new Java enrichment
  service wired the same way.

- **`go build ./...` / `go vet ./...` from the repo root fails with
  `directory prefix . does not contain modules listed in go.work`:** expected
  — see step 2, run per module instead.

- **`rpk topic describe`/`consume` run via `docker exec claimfraud-broker
  ...` fails to dial, or hangs, with something like `dial tcp
  [::1]:19092: connect: connection refused`:** the broker's advertised
  listener is `localhost:19092`, which only resolves correctly from the
  *host* (where the Docker port mapping lives) — from *inside* the
  container's own network namespace, `localhost:19092` doesn't exist
  (only `localhost:9092` does). `rpk topic list` works fine in-container
  (metadata-only), but anything needing a leader connection (`describe
  -p`, `consume`) follows the advertised address and breaks. Either point
  `rpk` at `localhost:9092` (the container-internal port) when running it
  *inside* the container via `docker exec`, or verify from the host
  instead — a real client (Orchestration, the test console, or a
  throwaway `kafka-go` consumer) connecting to `localhost:19092` from the
  host works correctly, since that's the address it's actually advertised
  for.
