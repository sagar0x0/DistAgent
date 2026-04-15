package registry

import (
	"errors"
	"sync"
	"time"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

// AgentRegistry tracks all connected Python CPU agents.
type AgentRegistry struct {
	mu      sync.RWMutex
	workers map[string]*WorkerEntry
	ttl     time.Duration
}

// WorkerEntry holds the latest heartbeat and last seen time.
type WorkerEntry struct {
	Heartbeat *distagentv1.HeartbeatRequest
	LastSeen  time.Time
}

func NewAgentRegistry(ttl time.Duration) *AgentRegistry {
	return &AgentRegistry{
		workers: make(map[string]*WorkerEntry),
		ttl:     ttl,
	}
}

func (r *AgentRegistry) RegisterOrUpdate(hb *distagentv1.HeartbeatRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.workers[hb.AgentId] = &WorkerEntry{
		Heartbeat: hb,
		LastSeen:  time.Now(),
	}
}

func (r *AgentRegistry) Remove(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.workers, agentID)
}

// GetLeastLoaded Agent finds an available agent to execute a task
func (r *AgentRegistry) GetLeastLoaded() (*distagentv1.HeartbeatRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	var best *distagentv1.HeartbeatRequest
	bestLoad := float64(-1)

	for id, entry := range r.workers {
		if now.Sub(entry.LastSeen) > r.ttl {
			// Expired, skip (cleanup should handle this eventually)
			continue
		}

		if entry.Heartbeat.Status != distagentv1.AgentStatus_AGENT_STATUS_READY {
			continue
		}

		// Simplified load calculation based on active tasks ratio
		var load float64
		if entry.Heartbeat.MaxConcurrentTasks > 0 {
			load = float64(entry.Heartbeat.ActiveTasks) / float64(entry.Heartbeat.MaxConcurrentTasks)
		} else {
            load = 1.0 // fallback
        }

		if load >= 1.0 {
			continue // Fully loaded
		}

		if bestLoad == -1 || load < bestLoad {
			bestLoad = load
			best = entry.Heartbeat
			_ = id
		}
	}

	if best == nil {
		return nil, errors.New("no available agent workers")
	}

	return best, nil
}
