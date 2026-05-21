package registry

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

func setupAgentRegistry(numAgents int) *AgentRegistry {
	reg := NewAgentRegistry(30 * time.Second)
	for i := 0; i < numAgents; i++ {
		reg.RegisterOrUpdate(&distagentv1.HeartbeatRequest{
			AgentId:            fmt.Sprintf("agent-%d", i),
			Status:             distagentv1.AgentStatus_AGENT_STATUS_READY,
			ActiveTasks:        int32(i % 5),
			MaxConcurrentTasks: 10,
		})
	}
	return reg
}

// BenchmarkAgentRegistry_GetLeastLoaded measures scaling of linear selection
func BenchmarkAgentRegistry_GetLeastLoaded(b *testing.B) {
	scales := []int{5, 10, 50, 100, 500}

	for _, scale := range scales {
		b.Run(fmt.Sprintf("%d_Agents", scale), func(b *testing.B) {
			reg := setupAgentRegistry(scale)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, _ = reg.GetLeastLoaded()
			}
		})
	}
}

// BenchmarkAgentRegistry_GetLeastLoaded_Parallel tests contention on RLock
func BenchmarkAgentRegistry_GetLeastLoaded_Parallel(b *testing.B) {
	reg := setupAgentRegistry(50)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = reg.GetLeastLoaded()
		}
	})
}

// BenchmarkAgentRegistry_RegisterOrUpdate_Parallel tests contention on Lock
func BenchmarkAgentRegistry_RegisterOrUpdate_Parallel(b *testing.B) {
	reg := NewAgentRegistry(30 * time.Second)
	agents := make([]*distagentv1.HeartbeatRequest, 50)
	for i := 0; i < 50; i++ {
		agents[i] = &distagentv1.HeartbeatRequest{
			AgentId:            fmt.Sprintf("agent-%d", i),
			Status:             distagentv1.AgentStatus_AGENT_STATUS_READY,
			ActiveTasks:        int32(i % 5),
			MaxConcurrentTasks: 10,
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		localRand := rand.New(rand.NewSource(time.Now().UnixNano()))
		for pb.Next() {
			idx := localRand.Intn(50)
			reg.RegisterOrUpdate(agents[idx])
		}
	})
}

// BenchmarkAgentRegistry_MixedReadWrite_Parallel tests real-world realistic contention (90% reads, 10% writes)
func BenchmarkAgentRegistry_MixedReadWrite_Parallel(b *testing.B) {
	reg := setupAgentRegistry(50)
	agents := make([]*distagentv1.HeartbeatRequest, 50)
	for i := 0; i < 50; i++ {
		agents[i] = &distagentv1.HeartbeatRequest{
			AgentId:            fmt.Sprintf("agent-%d", i),
			Status:             distagentv1.AgentStatus_AGENT_STATUS_READY,
			ActiveTasks:        int32(i % 5),
			MaxConcurrentTasks: 10,
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		localRand := rand.New(rand.NewSource(time.Now().UnixNano()))
		for pb.Next() {
			if localRand.Intn(10) == 0 { // 10% writes
				idx := localRand.Intn(50)
				reg.RegisterOrUpdate(agents[idx])
			} else { // 90% reads
				_, _ = reg.GetLeastLoaded()
			}
		}
	})
}

// BenchmarkInferenceRegistry_GetCacheStates tests traversing cache states
func BenchmarkInferenceRegistry_GetCacheStates(b *testing.B) {
	scales := []int{10, 50, 100}

	for _, scale := range scales {
		b.Run(fmt.Sprintf("%d_Backends", scale), func(b *testing.B) {
			reg := NewInferenceRegistry(30 * time.Second)
			for i := 0; i < scale; i++ {
				reg.RegisterOrUpdate(&distagentv1.ReportHealthRequest{
					BackendId:     fmt.Sprintf("sglang-%d", i),
					LoadedModelId: "gpt-4",
					Status:        distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY,
					CacheState: &distagentv1.CacheState{
						WarmPrefixHashes: []string{"abc", "def", "ghi"},
					},
				})
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = reg.GetCacheStates("gpt-4")
			}
		})
	}
}
