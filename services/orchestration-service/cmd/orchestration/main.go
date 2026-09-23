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

	"claimfraud/pkg/telemetry"
	orchestrationv1 "claimfraud/proto/gen/go/orchestration/v1"
	"claimfraud/services/orchestration-service/internal/client"
	"claimfraud/services/orchestration-service/internal/dag"
	"claimfraud/services/orchestration-service/internal/kafka"
	"claimfraud/services/orchestration-service/internal/server"

	"google.golang.org/grpc"
)

func main() {
	grpcAddr := getenv("ORCHESTRATION_GRPC_ADDR", ":9091")
	addressNormAddr := getenv("ADDRESS_NORMALIZATION_ADDR", "localhost:9092")
	modelAddr := getenv("MODEL_SERVICE_ADDR", "localhost:9093")
	tenantConfigAddr := getenv("TENANT_CONFIG_ADDR", "localhost:9094")
	claimantIDHashAddr := getenv("CLAIMANT_ID_HASHING_ADDR", "localhost:9095")
	policyLookupAddr := getenv("POLICY_LOOKUP_ADDR", "localhost:9096")
	kafkaBrokerAddr := getenv("KAFKA_BROKER_ADDR", "localhost:19092")
	dagConfigDir := getenv("DAG_CONFIG_DIR", "../../config/dag")

	logger := telemetry.NewLogger("orchestration-service")

	tenantConfig, err := client.DialTenantConfig(tenantConfigAddr, logger)
	if err != nil {
		log.Fatalf("dial tenant-config-svc at %s: %v", tenantConfigAddr, err)
	}
	defer tenantConfig.Close()

	addressNorm, err := client.DialAddressNorm(addressNormAddr, logger)
	if err != nil {
		log.Fatalf("dial address-normalization-svc at %s: %v", addressNormAddr, err)
	}
	defer addressNorm.Close()

	claimantIDHash, err := client.DialClaimantIDHash(claimantIDHashAddr, logger)
	if err != nil {
		log.Fatalf("dial claimant-id-hashing-svc at %s: %v", claimantIDHashAddr, err)
	}
	defer claimantIDHash.Close()

	policyLookup, err := client.DialPolicyLookup(policyLookupAddr, logger)
	if err != nil {
		log.Fatalf("dial policy-lookup-svc at %s: %v", policyLookupAddr, err)
	}
	defer policyLookup.Close()

	model, err := client.DialModel(modelAddr, logger)
	if err != nil {
		log.Fatalf("dial model-service at %s: %v", modelAddr, err)
	}
	defer model.Close()

	publisher := kafka.NewPublisher(kafkaBrokerAddr)
	defer publisher.Close()

	srv := &server.Server{
		Executor: &dag.Executor{
			Loader: &dag.Loader{
				TenantConfig: tenantConfig,
				ConfigDir:    dagConfigDir,
				Logger:       logger,
			},
			AddressNorm:    addressNorm,
			ClaimantIDHash: claimantIDHash,
			PolicyLookup:   policyLookup,
			Model:          model,
			Publisher:      publisher,
			Logger:         logger,
		},
	}

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("listen on %s: %v", grpcAddr, err)
	}

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(telemetry.UnaryServerInterceptor(logger)))
	orchestrationv1.RegisterOrchestrationServiceServer(grpcServer, srv)

	logger.Info("orchestration-service starting",
		"grpc_addr", grpcAddr, "address_norm_addr", addressNormAddr, "claimant_id_hash_addr", claimantIDHashAddr,
		"policy_lookup_addr", policyLookupAddr, "model_addr", modelAddr, "tenant_config_addr", tenantConfigAddr,
		"kafka_broker_addr", kafkaBrokerAddr, "dag_config_dir", dagConfigDir)
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
