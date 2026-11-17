# See services/claims-gateway/Dockerfile for the build-context rationale
# (go.work requires the whole workspace) and the proto/gen/go gitignore
# caveat.
FROM golang:1.25 AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY pkg/telemetry ./pkg/telemetry
COPY proto/gen/go ./proto/gen/go
COPY services/claims-gateway ./services/claims-gateway
COPY services/model-service ./services/model-service
COPY services/orchestration-service ./services/orchestration-service
COPY services/tenant-config-svc ./services/tenant-config-svc
RUN CGO_ENABLED=0 go build -o /out/tenantconfig ./services/tenant-config-svc/cmd/tenantconfig

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/tenantconfig /tenantconfig
# TENANTS_FILE default in main.go is a dev-only relative path
# ("../../config/tenants.yaml") — the compose file overrides it to this
# absolute in-image path instead.
COPY config/tenants.yaml /app/config/tenants.yaml
EXPOSE 9094 9194
ENTRYPOINT ["/tenantconfig"]
