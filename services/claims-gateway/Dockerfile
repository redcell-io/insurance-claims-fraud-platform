# Build context is the repo root (see docker-compose.deploy.yml), not
# this directory — claimfraud/* imports only resolve via the go.work
# workspace, so the whole workspace (pkg/telemetry, proto/gen/go, and
# all four Go services it lists under `use`) has to be present even
# though this image only builds claims-gateway.
#
# NOTE: proto/gen/go is gitignored (regenerated via proto/generate.sh,
# not committed) — it must already exist on disk before `docker build`/
# `docker compose build` runs. A fresh `git clone` won't have it.
FROM golang:1.25 AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY pkg/telemetry ./pkg/telemetry
COPY proto/gen/go ./proto/gen/go
COPY services/claims-gateway ./services/claims-gateway
COPY services/model-service ./services/model-service
COPY services/orchestration-service ./services/orchestration-service
COPY services/tenant-config-svc ./services/tenant-config-svc
RUN CGO_ENABLED=0 go build -o /out/gateway ./services/claims-gateway/cmd/gateway

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/gateway /gateway
EXPOSE 8080
ENTRYPOINT ["/gateway"]
