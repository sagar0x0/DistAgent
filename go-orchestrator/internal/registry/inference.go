package registry

import (
	"sync"
	"time"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

// InferenceRegistry tracks all GPU inference backends (like SGLang pods).
type InferenceRegistry struct {
	mu       sync.RWMutex
	backends map[string]*BackendEntry
	ttl      time.Duration
}

// BackendEntry holds the latest heartbeat and last seen time.
type BackendEntry struct {
	Heartbeat *distagentv1.ReportHealthRequest
	LastSeen  time.Time
}

func NewInferenceRegistry(ttl time.Duration) *InferenceRegistry {
	return &InferenceRegistry{
		backends: make(map[string]*BackendEntry),
		ttl:      ttl,
	}
}

func (r *InferenceRegistry) RegisterOrUpdate(hb *distagentv1.ReportHealthRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.backends[hb.BackendId] = &BackendEntry{
		Heartbeat: hb,
		LastSeen:  time.Now(),
	}
}

func (r *InferenceRegistry) Remove(backendID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.backends, backendID)
}

// GetByModel returns a list of active backends serving the requested model.
// This list is used by the KV-Cache Router (CHWBL) to select the best one.
func (r *InferenceRegistry) GetByModel(modelID string) []*distagentv1.ReportHealthRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	var available []*distagentv1.ReportHealthRequest

	for _, entry := range r.backends {
		if now.Sub(entry.LastSeen) > r.ttl {
			// Expired, skip
			continue
		}

		if entry.Heartbeat.Status != distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY {
			continue
		}

		if entry.Heartbeat.LoadedModelId == modelID {
			available = append(available, entry.Heartbeat)
		}
	}

	return available
}

// GetCacheStates returns a map of backendID to CacheState for the requested model.
// This allows the router to make prefix-aware routing decisions.
func (r *InferenceRegistry) GetCacheStates(modelID string) map[string]*distagentv1.CacheState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	states := make(map[string]*distagentv1.CacheState)

	for id, entry := range r.backends {
		if now.Sub(entry.LastSeen) > r.ttl {
			continue
		}
		if entry.Heartbeat.Status != distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY {
			continue
		}
		if entry.Heartbeat.LoadedModelId == modelID && entry.Heartbeat.CacheState != nil {
			states[id] = entry.Heartbeat.CacheState
		}
	}

	return states
}
