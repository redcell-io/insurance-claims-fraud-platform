// Command orchestration runs the Orchestration Service: a gRPC server
// that executes the enrichment DAG for a claim.
//
// Walking-skeleton scope only — the DAG is hardcoded (see internal/dag),
// not loaded from tenant config. See DESIGN.md §6.1 and build-order-plan
// for what's deferred.
package main

import (
	"log"
	"net"
	"os"

	orchestrationv1 "claimfraud/proto/gen/go/orchestration/v1"
	"claimfraud/services/orchestration-service/internal/client"
	"claimfraud/services/orchestration-service/internal/dag"
	"claimfraud/services/orchestration-service/internal/server"

	"google.golang.org/grpc"
)

func main() {
	grpcAddr := getenv("ORCHESTRATION_GRPC_ADDR", ":9091")
	addressNormAddr := getenv("ADDRESS_NORMALIZATION_ADDR", "localhost:9092")
	modelAddr := getenv("MODEL_SERVICE_ADDR", "localhost:9093")

	addressNorm, err := client.DialAddressNorm(addressNormAddr)
	if err != nil {
		log.Fatalf("dial address-normalization-svc at %s: %v", addressNormAddr, err)
	}
	defer addressNorm.Close()

	model, err := client.DialModel(modelAddr)
	if err != nil {
		log.Fatalf("dial model-service at %s: %v", modelAddr, err)
	}
	defer model.Close()

	srv := &server.Server{
		Executor: &dag.Executor{
			AddressNorm: addressNorm,
			Model:       model,
		},
	}

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("listen on %s: %v", grpcAddr, err)
	}

	grpcServer := grpc.NewServer()
	orchestrationv1.RegisterOrchestrationServiceServer(grpcServer, srv)

	log.Printf("orchestration-service listening on %s (address-norm=%s, model=%s)", grpcAddr, addressNormAddr, modelAddr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("orchestration-service server failed: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
