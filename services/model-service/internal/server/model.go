// Package server implements a STUB ModelService: returns a fixed score
// regardless of input. Real version (DESIGN.md §9): Java + ONNX Runtime
// or a Go client to TorchServe/Triton, feature-map-driven inference,
// per-tenant/product model_version resolution. See build-order-plan —
// this stub exists only to prove the gRPC contract end-to-end.
package server

import (
	"context"

	modelv1 "claimfraud/proto/gen/go/model/v1"
)

const (
	stubScore        = 0.42
	stubModelVersion = "stub-v0"
)

type Server struct {
	modelv1.UnimplementedModelServiceServer
}

func (s *Server) Score(ctx context.Context, req *modelv1.ScoreRequest) (*modelv1.ScoreResponse, error) {
	return &modelv1.ScoreResponse{
		Score:        stubScore,
		ModelVersion: stubModelVersion,
	}, nil
}
