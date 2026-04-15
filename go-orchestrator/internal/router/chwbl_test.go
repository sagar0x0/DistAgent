package router

import (
	"testing"
	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

func TestHashRing_ConsistentRouting(t *testing.T) {
	ring := NewHashRing()
	
	node1 := &distagentv1.ReportHealthRequest{BackendId: "sg-1", Status: distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY}
	node2 := &distagentv1.ReportHealthRequest{BackendId: "sg-2", Status: distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY}
	
	ring.AddBackend(node1)
	ring.AddBackend(node2)
	ring.Sort()
	
	// A specific session should map consistently to the same node
	targetA := ring.GetBackend("session-777").BackendId
	targetB := ring.GetBackend("session-777").BackendId
	
	if targetA != targetB {
		t.Errorf("expected consistent routing, got %s then %s", targetA, targetB)
	}
}

func TestHashRing_BoundedLoadSkip(t *testing.T) {
	ring := NewHashRing()
	
	// Force the optimal ring target to be busy
	nodeBusy := &distagentv1.ReportHealthRequest{BackendId: "sg-busy", Status: distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_BUSY}
	nodeReady := &distagentv1.ReportHealthRequest{BackendId: "sg-ready", Status: distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY}
	
	ring.AddBackend(nodeBusy)
	ring.AddBackend(nodeReady)
	ring.Sort()
	
	target := ring.GetBackend("session-777")
	
	// The routing should have bounded load skipped the busy node entirely
	if target.BackendId == "sg-busy" {
		t.Errorf("bounded load failed; router selected busy node over ready node")
	}
	if target.BackendId != "sg-ready" {
		t.Errorf("router failed to pick ready node, got %s", target.BackendId)
	}
}

// ---------------------------------------------------------------------------
// GetBackendWithCacheAffinity tests
// ---------------------------------------------------------------------------

// TestGetBackendWithCacheAffinity_CacheHit verifies that a READY backend
// reporting the requested prefix in its WarmPrefixHashes is selected over
// the ring's default CHWBL target.
func TestGetBackendWithCacheAffinity_CacheHit(t *testing.T) {
	ring := NewHashRing()

	sg0 := &distagentv1.ReportHealthRequest{BackendId: "sg-0", Status: distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY}
	sg1 := &distagentv1.ReportHealthRequest{BackendId: "sg-1", Status: distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY}
	sg2 := &distagentv1.ReportHealthRequest{BackendId: "sg-2", Status: distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY}

	ring.AddBackend(sg0)
	ring.AddBackend(sg1)
	ring.AddBackend(sg2)
	ring.Sort()

	prefixHash := "abc123prefix"

	// Only sg-2 has this prefix warm
	cacheStates := map[string]*distagentv1.CacheState{
		"sg-0": {WarmPrefixHashes: []string{"zzz000"}},
		"sg-1": {WarmPrefixHashes: []string{"yyy111"}},
		"sg-2": {WarmPrefixHashes: []string{"abc123prefix", "zzz000"}},
	}

	result := ring.GetBackendWithCacheAffinity(prefixHash, "session-x", cacheStates)
	if result == nil {
		t.Fatal("expected a backend, got nil")
	}
	if result.BackendId != "sg-2" {
		t.Errorf("expected cache-hit backend sg-2, got %s", result.BackendId)
	}
}

// TestGetBackendWithCacheAffinity_FallbackWhenNoCacheHit verifies that when
// no backend has the prefix warm the router still returns a READY backend
// (standard CHWBL fallback).
func TestGetBackendWithCacheAffinity_FallbackWhenNoCacheHit(t *testing.T) {
	ring := NewHashRing()

	sg0 := &distagentv1.ReportHealthRequest{BackendId: "sg-0", Status: distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY}
	sg1 := &distagentv1.ReportHealthRequest{BackendId: "sg-1", Status: distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY}

	ring.AddBackend(sg0)
	ring.AddBackend(sg1)
	ring.Sort()

	prefixHash := "brand-new-prefix"

	// No backend has this prefix warm
	cacheStates := map[string]*distagentv1.CacheState{
		"sg-0": {WarmPrefixHashes: []string{}},
		"sg-1": {WarmPrefixHashes: []string{}},
	}

	result := ring.GetBackendWithCacheAffinity(prefixHash, "session-y", cacheStates)
	if result == nil {
		t.Fatal("expected fallback backend, got nil")
	}
	// Should be deterministic — same prefix hash always lands the same fallback
	result2 := ring.GetBackendWithCacheAffinity(prefixHash, "session-y", cacheStates)
	if result.BackendId != result2.BackendId {
		t.Errorf("fallback is not deterministic: %s vs %s", result.BackendId, result2.BackendId)
	}
}

// TestGetBackendWithCacheAffinity_SkipsBusyEvenWithWarmPrefix verifies that
// a BUSY backend is never selected even if it has the warm prefix.
func TestGetBackendWithCacheAffinity_SkipsBusyEvenWithWarmPrefix(t *testing.T) {
	ring := NewHashRing()

	// sg-busy holds the prefix but is overloaded; sg-ready does not have it
	sgBusy  := &distagentv1.ReportHealthRequest{BackendId: "sg-busy",  Status: distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_BUSY}
	sgReady := &distagentv1.ReportHealthRequest{BackendId: "sg-ready", Status: distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY}

	ring.AddBackend(sgBusy)
	ring.AddBackend(sgReady)
	ring.Sort()

	prefixHash := "hot-prefix"

	cacheStates := map[string]*distagentv1.CacheState{
		"sg-busy":  {WarmPrefixHashes: []string{"hot-prefix"}},
		"sg-ready": {WarmPrefixHashes: []string{}},
	}

	result := ring.GetBackendWithCacheAffinity(prefixHash, "session-z", cacheStates)
	if result == nil {
		t.Fatal("expected a backend, got nil")
	}
	if result.BackendId == "sg-busy" {
		t.Errorf("router selected BUSY backend despite having warm prefix — bounded load violated")
	}
}

// TestGetBackendWithCacheAffinity_EmptyPrefixFallsBackToSessionID verifies
// that when no prefix hash is supplied the router falls back gracefully to
// session-ID-based CHWBL (backward compatibility).
func TestGetBackendWithCacheAffinity_EmptyPrefixFallsBackToSessionID(t *testing.T) {
	ring := NewHashRing()

	sg0 := &distagentv1.ReportHealthRequest{BackendId: "sg-0", Status: distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY}
	sg1 := &distagentv1.ReportHealthRequest{BackendId: "sg-1", Status: distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY}

	ring.AddBackend(sg0)
	ring.AddBackend(sg1)
	ring.Sort()

	// No prefix hash — empty string
	r1 := ring.GetBackendWithCacheAffinity("", "session-stable", nil)
	r2 := ring.GetBackendWithCacheAffinity("", "session-stable", nil)

	if r1 == nil || r2 == nil {
		t.Fatal("expected backend from session-ID fallback, got nil")
	}
	if r1.BackendId != r2.BackendId {
		t.Errorf("session-ID fallback not deterministic: %s vs %s", r1.BackendId, r2.BackendId)
	}
}

