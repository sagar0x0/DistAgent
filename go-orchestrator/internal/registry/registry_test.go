package registry

import (
    "testing"
    "time"

    distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

func TestAgentRegistry_GetLeastLoaded(t *testing.T) {
    reg := NewAgentRegistry(time.Minute)

    // No agents
    _, err := reg.GetLeastLoaded()
    if err == nil {
        t.Fatal("expected error with empty registry")
    }

    // Add Agent 1 (Busy)
    reg.RegisterOrUpdate(&distagentv1.HeartbeatRequest{
        AgentId:            "agent-1",
        Status:             distagentv1.AgentStatus_AGENT_STATUS_READY,
        ActiveTasks:        10,
        MaxConcurrentTasks: 10,
    })

    // Add Agent 2 (Available, Load 50%)
    reg.RegisterOrUpdate(&distagentv1.HeartbeatRequest{
        AgentId:            "agent-2",
        Status:             distagentv1.AgentStatus_AGENT_STATUS_READY,
        ActiveTasks:        5,
        MaxConcurrentTasks: 10,
    })

    // Add Agent 3 (Available, Load 10%)
    reg.RegisterOrUpdate(&distagentv1.HeartbeatRequest{
        AgentId:            "agent-3",
        Status:             distagentv1.AgentStatus_AGENT_STATUS_READY,
        ActiveTasks:        1,
        MaxConcurrentTasks: 10,
    })

    best, err := reg.GetLeastLoaded()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    if best.AgentId != "agent-3" {
        t.Fatalf("expected agent-3, got %s", best.AgentId)
    }
}

func TestInferenceRegistry_GetByModel(t *testing.T) {
    reg := NewInferenceRegistry(time.Minute)

    reg.RegisterOrUpdate(&distagentv1.ReportHealthRequest{
        BackendId:     "sglang-0",
        Status:        distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY,
        LoadedModelId: "llama-70b",
        EndpointUrl:   "http://sglang-0/v1",
    })

    reg.RegisterOrUpdate(&distagentv1.ReportHealthRequest{
        BackendId:     "sglang-1",
        Status:        distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_BUSY, // Not ready
        LoadedModelId: "llama-70b",
        EndpointUrl:   "http://sglang-1/v1",
    })

    reg.RegisterOrUpdate(&distagentv1.ReportHealthRequest{
        BackendId:     "sglang-2",
        Status:        distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY,
        LoadedModelId: "gpt-4o", // Different model
        EndpointUrl:   "http://sglang-2/v1",
    })

    // Query for llama-70b
    backends := reg.GetByModel("llama-70b")
    if len(backends) != 1 {
        t.Fatalf("expected 1 backend, got %d", len(backends))
    }
    if backends[0].BackendId != "sglang-0" {
        t.Fatalf("expected sglang-0, got %s", backends[0].BackendId)
    }

    // Query for gpt-4o
    backends = reg.GetByModel("gpt-4o")
    if len(backends) != 1 {
        t.Fatalf("expected 1 backend for gpt-4o")
    }

    // Query for unknown
    backends = reg.GetByModel("unknown")
    if len(backends) != 0 {
        t.Fatalf("expected 0 backends for unknown model")
    }
}
