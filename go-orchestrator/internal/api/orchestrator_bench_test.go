package api

import (
	"fmt"
	"testing"
	"time"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
	"github.com/sagar/distagent/internal/registry"
	"github.com/sagar/distagent/internal/router"
)

func setupControlPlane() (*registry.AgentRegistry, *router.InferenceRouter) {
	// 1. Setup Agent Registry with 50 agents
	agentReg := registry.NewAgentRegistry(30 * time.Second)
	for i := 0; i < 50; i++ {
		agentReg.RegisterOrUpdate(&distagentv1.HeartbeatRequest{
			AgentId:            fmt.Sprintf("agent-%d", i),
			Status:             distagentv1.AgentStatus_AGENT_STATUS_READY,
			ActiveTasks:        int32(i % 5),
			MaxConcurrentTasks: 10,
		})
	}

	// 2. Setup Inference Registry with 10 backends
	infReg := registry.NewInferenceRegistry(30 * time.Second)
	for i := 0; i < 10; i++ {
		infReg.RegisterOrUpdate(&distagentv1.ReportHealthRequest{
			BackendId:     fmt.Sprintf("sglang-%d", i),
			Status:        distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY,
			LoadedModelId: "llama-3-8b",
			CacheState: &distagentv1.CacheState{
				WarmPrefixHashes: []string{"hot-prefix-123"},
			},
		})
	}

	infRouter := router.NewInferenceRouter(infReg)

	return agentReg, infRouter
}

// BenchmarkControlPlane_FullRoutingDecision measures the complete decision path
// PrefixHash -> CHWBL Inference Routing -> Agent Least-Load Selection
func BenchmarkControlPlane_FullRoutingDecision(b *testing.B) {
	agentReg, infRouter := setupControlPlane()
	
	systemPrompt := "You are a master planner AI."
	tools := []*distagentv1.ToolDefinition{
		{Name: "search", ParametersJsonSchema: `{"type":"object"}`},
		{Name: "weather", ParametersJsonSchema: `{"type":"object"}`},
	}
	sessionID := "session-bench"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// 1. Compute Prefix Hash
		prefixHash := router.ComputePrefixHash(systemPrompt, tools)
		
		// 2. CHWBL Inference Routing
		_, err := infRouter.Route("llama-3-8b", sessionID, prefixHash)
		if err != nil {
			b.Fatal(err)
		}
		
		// 3. Agent Selection
		_, err = agentReg.GetLeastLoaded()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkControlPlane_FullRoutingDecision_Parallel tests the hot path under contention
func BenchmarkControlPlane_FullRoutingDecision_Parallel(b *testing.B) {
	agentReg, infRouter := setupControlPlane()
	
	systemPrompt := "You are a master planner AI."
	tools := []*distagentv1.ToolDefinition{
		{Name: "search", ParametersJsonSchema: `{"type":"object"}`},
		{Name: "weather", ParametersJsonSchema: `{"type":"object"}`},
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sessionID := fmt.Sprintf("session-%d", i)
			prefixHash := router.ComputePrefixHash(systemPrompt, tools)
			
			_, _ = infRouter.Route("llama-3-8b", sessionID, prefixHash)
			_, _ = agentReg.GetLeastLoaded()
			i++
		}
	})
}
