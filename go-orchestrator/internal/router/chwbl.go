package router

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

// Define basic Consistent Hashing with Bounded Loads (CHWBL) structure
type HashRing struct {
	nodes []uint32
	keys  map[uint32]*distagentv1.ReportHealthRequest
}

func NewHashRing() *HashRing {
	return &HashRing{
		keys: make(map[uint32]*distagentv1.ReportHealthRequest),
	}
}

func hashKey(key string) uint32 {
	h := sha256.New()
	h.Write([]byte(key))
	sum := h.Sum(nil)
	return binary.BigEndian.Uint32(sum[:4])
}

// AddBackend adds an inference node to the hash ring using virtual nodes for better distribution
func (r *HashRing) AddBackend(backend *distagentv1.ReportHealthRequest) {
	vNodes := 10 // Virtual node amplification
	for i := 0; i < vNodes; i++ {
		vKey := fmt.Sprintf("%s-v%d", backend.BackendId, i)
		hash := hashKey(vKey)
		r.nodes = append(r.nodes, hash)
		r.keys[hash] = backend
	}
}

// Sort ensures the ring is sorted properly for binary searching
func (r *HashRing) Sort() {
	sort.Slice(r.nodes, func(i, j int) bool {
		return r.nodes[i] < r.nodes[j]
	})
}

// GetBackend searches for the target backend and implements bounded loads
func (r *HashRing) GetBackend(sessionID string) *distagentv1.ReportHealthRequest {
	if len(r.nodes) == 0 {
		return nil
	}
	
	hash := hashKey(sessionID)
	
	// Binary search for closest node
	idx := sort.Search(len(r.nodes), func(i int) bool {
		return r.nodes[i] >= hash
	})
	
	if idx == len(r.nodes) {
		idx = 0 // Wrap around
	}
	
	// Bounded Load Logic:
	// Once we hit the designated ring target, we check its status. 
	// If it's BUSY, we probe the next virtual node in the ring.
	maxProbes := 15
	probes := 0
	
	currIdx := idx
	for probes < maxProbes && probes < len(r.nodes) {
		targetBackend := r.keys[r.nodes[currIdx]]
		
		// Load bounding logic: is it accepting connections?
		if targetBackend.Status == distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY {
			// Found a healthy target
			return targetBackend
		}
		
		currIdx++
		if currIdx >= len(r.nodes) {
			currIdx = 0 
		}
		probes++
	}
	
	// Fallback to initial target if all probed are busy (SGLang internal queues will catch it)
	return r.keys[r.nodes[idx]]
}

// GetBackendWithCacheAffinity searches for a backend with the desired cache prefix, or falls back to consistent hashing.
func (r *HashRing) GetBackendWithCacheAffinity(prefixHash string, sessionID string, cacheStates map[string]*distagentv1.CacheState) *distagentv1.ReportHealthRequest {
	if len(r.nodes) == 0 {
		return nil
	}
	
	// 1. Try to find a backend that already has this prefix hash warm
	if prefixHash != "" {
		hash := hashKey(prefixHash)
		idx := sort.Search(len(r.nodes), func(i int) bool {
			return r.nodes[i] >= hash
		})
		if idx == len(r.nodes) {
			idx = 0 // Wrap around
		}

		maxProbes := 3 // Short probe range for exact cache hits
		currIdx := idx
		for probes := 0; probes < maxProbes && probes < len(r.nodes); probes++ {
			targetBackend := r.keys[r.nodes[currIdx]]
			
			if targetBackend.Status == distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY {
				if state, ok := cacheStates[targetBackend.BackendId]; ok && state != nil {
					for _, warmHash := range state.WarmPrefixHashes {
						if warmHash == prefixHash {
							return targetBackend
						}
					}
				}
			}
			
			currIdx++
			if currIdx >= len(r.nodes) {
				currIdx = 0
			}
		}
	}

	// 2. Fallback to standard CHWBL using prefixHash as primary key (or sessionID if empty)
	hashTarget := prefixHash
	if hashTarget == "" {
		hashTarget = sessionID
	}
	
	hash := hashKey(hashTarget)
	idx := sort.Search(len(r.nodes), func(i int) bool {
		return r.nodes[i] >= hash
	})
	if idx == len(r.nodes) {
		idx = 0 // Wrap around
	}

	maxProbes := 15
	currIdx := idx
	for probes := 0; probes < maxProbes && probes < len(r.nodes); probes++ {
		targetBackend := r.keys[r.nodes[currIdx]]
		if targetBackend.Status == distagentv1.InferenceBackendStatus_INFERENCE_BACKEND_STATUS_READY {
			return targetBackend
		}
		
		currIdx++
		if currIdx >= len(r.nodes) {
			currIdx = 0 
		}
	}
	
	return r.keys[r.nodes[idx]]
}
