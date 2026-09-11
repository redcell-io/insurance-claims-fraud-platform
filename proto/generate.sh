#!/usr/bin/env bash
# Regenerates Go protobuf/gRPC stubs from proto/*.proto into proto/gen/go/.
#
# Requires: tools/protoc/bin/protoc.exe (portable, see README.md) and the
# protoc-gen-go / protoc-gen-go-grpc plugins on PATH:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
#
# Java stubs are NOT generated here — each Java service generates its own
# via the protobuf-maven-plugin at build time (see
# services/address-normalization-svc/pom.xml), which also auto-fetches its
# own protoc, so it doesn't depend on tools/protoc/.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

export PATH="$PATH:$(go env GOPATH)/bin"
PROTOC="tools/protoc/bin/protoc.exe"

"$PROTOC" -I proto \
  --go_out=proto/gen/go --go_opt=paths=source_relative \
  --go-grpc_out=proto/gen/go --go-grpc_opt=paths=source_relative \
  proto/claims/v1/claims.proto \
  proto/orchestration/v1/orchestration.proto \
  proto/addressnorm/v1/address_normalization.proto \
  proto/model/v1/model.proto \
  proto/tenantconfig/v1/tenant_config.proto \
  proto/claimantidhash/v1/claimant_id_hashing.proto \
  proto/policylookup/v1/policy_lookup.proto

echo "Generated Go stubs into proto/gen/go/"
