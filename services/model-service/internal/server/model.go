// Package server implements ModelService: real, deterministic
// feature-driven scoring, not ML yet. Real version (DESIGN.md §9): Java +
// ONNX Runtime or a Go client to TorchServe/Triton, a trained model
// artifact, per-tenant/product model_version resolution. See DECISIONS.md
// #16 for why this thin slice is a formula rather than ONNX, and what's
// deferred.
package server

import (
	"context"
	"log/slog"

	modelv1 "claimfraud/proto/gen/go/model/v1"

	"claimfraud/pkg/telemetry"
	"claimfraud/services/model-service/internal/scoring"
)

type Server struct {
	modelv1.UnimplementedModelServiceServer
	Logger *slog.Logger
}

func (s *Server) Score(ctx context.Context, req *modelv1.ScoreRequest) (*modelv1.ScoreResponse, error) {
	score, explanations := scoring.Score(req.GetFeatures())

	telemetry.FromContext(ctx, s.Logger).Info("scored claim", "score", score, "reasons", explanations)

	return &modelv1.ScoreResponse{
		Score:        score,
		ModelVersion: scoring.ModelVersion,
	}, nil
}
