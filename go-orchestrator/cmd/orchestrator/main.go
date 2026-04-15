package main

import (
	"log"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
	"github.com/sagar/distagent/gen/distagent/v1/distagentv1connect"
	"github.com/sagar/distagent/internal/api"
	"github.com/sagar/distagent/internal/registry"
	"github.com/sagar/distagent/internal/router"
)

func main() {
	// 1. Core Services Setup
	agentReg := registry.NewAgentRegistry(30 * time.Second)
	infReg := registry.NewInferenceRegistry(30 * time.Second)

	infRouter := router.NewInferenceRouter(infReg)

	// 2. Client-Facing API (ConnectRPC over HTTP)
	// Supports REST + gRPC interchangeably on port 8080
	mux := http.NewServeMux()
	path, handler := distagentv1connect.NewOrchestratorServiceHandler(
		api.NewOrchestratorServer(agentReg, infRouter),
	)
	mux.Handle(path, handler)

	// 3. Internal Pure-gRPC Server for Python component connectivity (Heartbeats)
	lis, err := net.Listen("tcp", ":8081")
	if err != nil {
		log.Fatalf("failed to listen on :8081: %v", err)
	}
	grpcServer := grpc.NewServer()
	
	// Mount the heartbeat server that mutates the central AgentRegistry
	distagentv1.RegisterAgentWorkerServiceServer(grpcServer, api.NewAgentHeartbeatServer(agentReg))

	// Start pure gRPC background loop
	go func() {
		log.Println("Internal gRPC running (Agents Dial :8081)")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("grpcServer failed: %v", err)
		}
	}()

	// Start frontend HTTP gateway
	log.Println("External ConnectRPC Gateway ready on :8080")
	err = http.ListenAndServe(
		":8080",
		// Allow HTTP2 traversal without TLS internally (Ingress handles TLS edge)
		h2c.NewHandler(mux, &http2.Server{}),
	)
	log.Fatalf("http server failed: %v", err)
}
