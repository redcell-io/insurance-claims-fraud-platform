// Command model runs Model Service — real, deterministic feature-driven
// scoring (not ML yet). See internal/server and DECISIONS.md #16.
package main

import (
	"log"
	"net"
	"os"

	"claimfraud/pkg/telemetry"
	modelv1 "claimfraud/proto/gen/go/model/v1"
	"claimfraud/services/model-service/internal/server"

	"google.golang.org/grpc"
)

func main() {
	grpcAddr := getenv("MODEL_GRPC_ADDR", ":9093")

	logger := telemetry.NewLogger("model-service")

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("listen on %s: %v", grpcAddr, err)
	}

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(telemetry.UnaryServerInterceptor(logger)))
	modelv1.RegisterModelServiceServer(grpcServer, &server.Server{Logger: logger})

	logger.Info("model-service starting", "grpc_addr", grpcAddr, "scoring_mode", "rules-based")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("model-service server failed: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
