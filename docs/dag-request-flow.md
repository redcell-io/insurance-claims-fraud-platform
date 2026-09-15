# Request → DAG call chain

Traces exactly what calls what for a single `POST /v1/claims`, file by file.
Useful alongside [config.go](../services/orchestration-service/internal/dag/config.go)/
[loader.go](../services/orchestration-service/internal/dag/loader.go)/
[executor.go](../services/orchestration-service/internal/dag/executor.go) —
those three answer "how does a DAG get shaped, loaded, and run"; this answers
"where does execution actually go, in order, across services."

## Sequence

```mermaid
sequenceDiagram
    participant Client
    participant GW as ClaimsGateway
    participant TC as tenant-config-svc
    participant Orch as Orchestration
    participant CIH as claimant-id-hashing-svc
    participant AN as address-normalization-svc
    participant PL as policy-lookup-svc
    participant Model as model-service
    participant Kafka as claims.realtime

    Client->>GW: POST /v1/claims
    GW->>TC: GetTenant(hardcodedTenantID)
    TC-->>GW: TenantContext{status}
    Note over GW: 403 if status != active, stop here
    GW->>Orch: ProcessClaim(claim) [gRPC, tenant_id/correlation_id in metadata]
    Orch->>TC: GetDagVersion(tenant_id, product)
    TC-->>Orch: version
    Note over Orch: reads config/dag/{tenant}/{product}/{event_type}.yaml<br/>from disk directly (not via a service call)
    par enrichment stage (all depends_on: none)
        Orch->>CIH: HashClaimantId(claimant_name) [on_failure: fail_fast]
        CIH-->>Orch: claimant_id_hash
    and
        Orch->>AN: Normalize(raw_address) [on_failure: skip]
        AN-->>Orch: normalized_address
    and
        Orch->>PL: LookupPolicy(policy_number) [on_failure: degrade]
        PL-->>Orch: found, status, coverage_type
    end
    Note over Orch: features use claimant_id_hash, never raw claimant_name
    Orch->>Model: Score(features) [depends_on: enrichment]
    Model-->>Orch: fraud_score, model_version
    Orch->>Kafka: publish ScoredClaimEvent (JSON), key=tenant_id [depends_on: model_score, on_failure: skip]
    Note over Orch: broker outage degrades the response, doesn't fail it
    Orch-->>GW: ProcessClaimResponse
    GW-->>Client: 200 JSON response
```

(The `par` block reflects the DAG's *declared* concurrency — nodes with no
`depends_on` are logically independent. The current executor actually runs
them sequentially within that stage; see `executor.go`'s doc comment and
DECISIONS.md #10 for why that's a deliberate, not-yet-needed simplification.)

## File-by-file

| Step | What happens | File |
|---|---|---|
| 1 | REST endpoint registered, dials downstream services | [cmd/gateway/main.go](../services/claims-gateway/cmd/gateway/main.go) |
| 2 | `SubmitClaim` handler entry point; generates `correlation_id` | [handler/claims.go](../services/claims-gateway/internal/handler/claims.go) |
| 3 | `resolveTenant()` → gRPC call to tenant-config-svc; `403` if not `active` | [handler/claims.go](../services/claims-gateway/internal/handler/claims.go) → [client/tenantconfig.go](../services/claims-gateway/internal/client/tenantconfig.go) |
| 4 | `TenantConfigService.GetTenant` server-side, looks up in-memory store | [tenant-config-svc/internal/server/tenantconfig.go](../services/tenant-config-svc/internal/server/tenantconfig.go) → [internal/store/store.go](../services/tenant-config-svc/internal/store/store.go) |
| 5 | Forward to Orchestration over gRPC, `tenant_id`/`correlation_id` as metadata | [client/orchestration.go](../services/claims-gateway/internal/client/orchestration.go) |
| 6 | `ProcessClaim` server entry, reads `tenant_id` from metadata, calls the executor | [orchestration-service/internal/server/orchestration.go](../services/orchestration-service/internal/server/orchestration.go) |
| 7 | `Executor.Run` — loads the DAG, then runs it stage by stage | [internal/dag/executor.go](../services/orchestration-service/internal/dag/executor.go) |
| 8 | `Loader.Load` — resolves version via tenant-config-svc, reads the YAML file | [internal/dag/loader.go](../services/orchestration-service/internal/dag/loader.go) → [client/tenantconfig.go](../services/orchestration-service/internal/client/tenantconfig.go) |
| 9 | The DAG definition itself (data, not code) | `config/dag/{tenant_id}/{product}/{event_type}.yaml`, e.g. [config/dag/acme_insurance/auto/claim.fnol.yaml](../config/dag/acme_insurance/auto/claim.fnol.yaml) |
| 10 | YAML shape → Go structs (`Config`, `Node`) | [internal/dag/config.go](../services/orchestration-service/internal/dag/config.go) |
| 11a | Stage 1 (enrichment group): hash claimant ID | [internal/dag/executor.go](../services/orchestration-service/internal/dag/executor.go) → [client/claimantidhash.go](../services/orchestration-service/internal/client/claimantidhash.go) → (Java) [grpc/ClaimantIdHashingGrpcService.java](../services/claimant-id-hashing-svc/src/main/java/io/redcell/claimantidhash/grpc/ClaimantIdHashingGrpcService.java) → [ClaimantIdHasher.java](../services/claimant-id-hashing-svc/src/main/java/io/redcell/claimantidhash/ClaimantIdHasher.java) |
| 11b | Stage 1 (enrichment group): normalize address | [internal/dag/executor.go](../services/orchestration-service/internal/dag/executor.go) → [client/addressnorm.go](../services/orchestration-service/internal/client/addressnorm.go) → (Java) [grpc/AddressNormalizationGrpcService.java](../services/address-normalization-svc/src/main/java/io/redcell/addressnorm/grpc/AddressNormalizationGrpcService.java) → [AddressNormalizer.java](../services/address-normalization-svc/src/main/java/io/redcell/addressnorm/AddressNormalizer.java) |
| 11c | Stage 1 (enrichment group): look up policy | [internal/dag/executor.go](../services/orchestration-service/internal/dag/executor.go) → [client/policylookup.go](../services/orchestration-service/internal/client/policylookup.go) → (Java) [grpc/PolicyLookupGrpcService.java](../services/policy-lookup-svc/src/main/java/io/redcell/policylookup/grpc/PolicyLookupGrpcService.java) → [PolicyLookup.java](../services/policy-lookup-svc/src/main/java/io/redcell/policylookup/PolicyLookup.java) |
| 12 | Stage 2 (`depends_on: [enrichment]`): builds the feature map (uses `claimant_id_hash`, never raw `claimant_name` — DECISIONS.md #14) and calls Model Service | [internal/dag/executor.go](../services/orchestration-service/internal/dag/executor.go) → [client/model.go](../services/orchestration-service/internal/client/model.go) → (Go stub) [model-service/internal/server/model.go](../services/model-service/internal/server/model.go) |
| 13 | Stage 3 (`depends_on: [model_score]`): real publish to `claims.realtime` (JSON, keyed by `tenant_id`), `on_failure: skip` | [internal/dag/executor.go](../services/orchestration-service/internal/dag/executor.go) → [internal/kafka/publisher.go](../services/orchestration-service/internal/kafka/publisher.go) → Redpanda (`docker-compose.yml`) |
| 14 | Response built and returned up the chain | [orchestration-service/internal/server/orchestration.go](../services/orchestration-service/internal/server/orchestration.go) → [claims-gateway/internal/handler/claims.go](../services/claims-gateway/internal/handler/claims.go) |

## Notes

- Steps 3-4 and 6-13 are two separate gRPC hops (ClaimsGateway→Orchestration,
  Orchestration→{tenant-config-svc,claimant-id-hash,address-norm,policy-lookup,model})
  — Orchestration is the only service that talks to tenant-config-svc *and*
  the enrichment/model services; ClaimsGateway only ever talks to
  tenant-config-svc and Orchestration.
- Step 9 is data, not code — to change what the DAG does, edit the YAML, not
  `executor.go`. Step 10 is the Go type the YAML is parsed into. Step
  7/11a-13 is what actually walks that parsed `Config` and dispatches per
  node (`node.Service` string → concrete client call — see the `switch`
  blocks in `executor.go`).
- Steps 11a-11c are three separate nodes in the same "enrichment" group
  (none `depends_on` another) — declared as concurrent in the DAG's own
  model, but the current executor runs them sequentially in file order; see
  `executor.go`'s doc comment.
- This trace is current as of the real-Kafka-publish pass; `DECISIONS.md`
  explains why the DAG loading here is un-cached, un-validated, and
  staged rather than topologically sorted, why claimant hashing/policy
  lookup are thin slices (#12, #13), and why the Kafka publish is JSON to
  a Redpanda broker rather than Protobuf + schema registry (#15).
