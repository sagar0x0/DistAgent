package api

import (
	"io"
	"log"

	"github.com/sagar/distagent/gen/distagent/v1"
	"github.com/sagar/distagent/internal/registry"
)

// AgentHeartbeatServer handles agents dialing in securely
type AgentHeartbeatServer struct {
	distagentv1.UnimplementedAgentWorkerServiceServer
	registry *registry.AgentRegistry
}

func NewAgentHeartbeatServer(reg *registry.AgentRegistry) *AgentHeartbeatServer {
	return &AgentHeartbeatServer{registry: reg}
}

func (s *AgentHeartbeatServer) Heartbeat(stream distagentv1.AgentWorkerService_HeartbeatServer) error {
	log.Println("[Heartbeat] Stream established with an agent")
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		s.registry.RegisterOrUpdate(req)
		
		log.Printf("[Heartbeat] Agent '%s' | Load: %d/%d tasks | CPU: %.1f%%", 
			req.AgentId, req.ActiveTasks, req.MaxConcurrentTasks, req.CpuUsagePercent)

		err = stream.Send(&distagentv1.HeartbeatResponse{
			Accepted: true,
			Message:  "Orchestrator Ack",
		})
		if err != nil {
			return err
		}
	}
}
