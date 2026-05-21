package router

import (
	"fmt"
	"testing"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

func setupHashRing(numBackends int) *HashRing {
	ring := NewHashRing()
	for i := 0; i < numBackends; i++ {
		ring.AddBackend(&distagentv1.ReportHealthRequest{
			BackendId: fmt.Sprintf("sglang-%d", i),
			Status:    distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY,
		})
	}
	ring.Sort()
	return ring
}

// BenchmarkHashRing_GetBackend measures standard CHWBL lookup scaling
func BenchmarkHashRing_GetBackend(b *testing.B) {
	scales := []int{3, 10, 50, 100, 500}
	
	for _, scale := range scales {
		b.Run(fmt.Sprintf("%d_Backends", scale), func(b *testing.B) {
			ring := setupHashRing(scale)
			sessionID := "session-abc-123"
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				ring.GetBackend(sessionID)
			}
		})
	}
}

// BenchmarkHashRing_GetBackendWithCacheAffinity measures prefix-aware lookup scaling
func BenchmarkHashRing_GetBackendWithCacheAffinity(b *testing.B) {
	scales := []int{3, 10, 50, 100}
	prefixHash := "abcd1234efgh5678"
	sessionID := "session-abc"
	
	for _, scale := range scales {
		b.Run(fmt.Sprintf("%d_Backends", scale), func(b *testing.B) {
			ring := setupHashRing(scale)
			
			// Simulate 50% of backends having the prefix
			cacheStates := make(map[string]*distagentv1.CacheState)
			for i := 0; i < scale; i++ {
				if i%2 == 0 {
					cacheStates[fmt.Sprintf("sglang-%d", i)] = &distagentv1.CacheState{
						WarmPrefixHashes: []string{"other1", prefixHash, "other2"},
					}
				}
			}
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				ring.GetBackendWithCacheAffinity(prefixHash, sessionID, cacheStates)
			}
		})
	}
}

// BenchmarkHashRing_CacheHitVsMiss compares latency of hits vs fallback misses
func BenchmarkHashRing_CacheHitVsMiss(b *testing.B) {
	ring := setupHashRing(10)
	prefixHash := "hot-prefix"
	
	cacheStatesHit := map[string]*distagentv1.CacheState{
		"sglang-5": {WarmPrefixHashes: []string{prefixHash}},
	}
	cacheStatesMiss := map[string]*distagentv1.CacheState{}
	
	b.Run("CacheHit", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ring.GetBackendWithCacheAffinity(prefixHash, "session-1", cacheStatesHit)
		}
	})
	
	b.Run("CacheMiss", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ring.GetBackendWithCacheAffinity(prefixHash, "session-1", cacheStatesMiss)
		}
	})
}

// BenchmarkHashRing_AddBackend measures the cost of building the ring
func BenchmarkHashRing_AddBackend(b *testing.B) {
	scales := []int{10, 100, 500}
	
	for _, scale := range scales {
		b.Run(fmt.Sprintf("%d_Backends", scale), func(b *testing.B) {
			backends := make([]*distagentv1.ReportHealthRequest, scale)
			for i := 0; i < scale; i++ {
				backends[i] = &distagentv1.ReportHealthRequest{
					BackendId: fmt.Sprintf("sg-%d", i),
					Status:    distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY,
				}
			}
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				ring := NewHashRing()
				for j := 0; j < scale; j++ {
					ring.AddBackend(backends[j])
				}
				ring.Sort()
			}
		})
	}
}
