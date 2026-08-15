// Command model runs a STUB Model Service — fixed score, no real
// inference. See internal/server for why.
package main

import (
	"log"
	"net"
	"os"

	modelv1 "claimfraud/proto/gen/go/model/v1"
	"claimfraud/services/model-service/internal/server"

	"google.golang.org/grpc"
)

func main() {
	grpcAddr := getenv("MODEL_GRPC_ADDR", ":9093")

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("listen on %s: %v", grpcAddr, err)
	}

	grpcServer := grpc.NewServer()
	modelv1.RegisterModelServiceServer(grpcServer, &server.Server{})

	log.Printf("model-service (STUB) listening on %s", grpcAddr)
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
