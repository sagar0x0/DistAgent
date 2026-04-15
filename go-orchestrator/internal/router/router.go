package router

import (
	"errors"
	"fmt"
	"os"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
	"github.com/sagar/distagent/internal/registry"
)

type InferenceRouter struct {
	inferenceReg *registry.InferenceRegistry
}

func NewInferenceRouter(reg *registry.InferenceRegistry) *InferenceRouter {
	return &InferenceRouter{inferenceReg: reg}
}

// Route returns the appropriate endpoint configuration for a requested model.
// This natively bypasses any proxy architectures to ensure pure security.
func (r *InferenceRouter) Route(modelID string, sessionID string, prefixHash string) (*distagentv1.InferenceConfig, error) {
	// Native external routing (e.g. OpenAI) using raw secure endpoints
	if modelID == "gpt-4" || modelID == "gpt-4o" || modelID == "gpt-3.5-turbo" {
		return &distagentv1.InferenceConfig{
			EndpointUrl: "https://api.openai.com/v1",
			ModelId:     modelID,
			ApiKey:      os.Getenv("OPENAI_API_KEY"),
			Temperature: 0,
			PrefixHash:  prefixHash,
		}, nil
	}

	if modelID == "gemini-3.1-flash-lite-preview" {
		return &distagentv1.InferenceConfig{
			EndpointUrl: "https://generativelanguage.googleapis.com/v1beta/openai/",
			ModelId:     modelID,
			ApiKey:      os.Getenv("GEMINI_API_KEY"),
			Temperature: 0,
			PrefixHash:  prefixHash,
		}, nil
	}

	if modelID == "claude-3-5" || modelID == "claude-3-opus" {
		return nil, errors.New("anthropic direct mapping requires custom endpoints; use openai models for default bypass tracking")
	}

	// Dynamic local routing (e.g. SGLang self-hosted models in Kubernetes)
	// Uses the active backend clusters monitored dynamically
	backends := r.inferenceReg.GetByModel(modelID)
	if len(backends) == 0 {
		return nil, fmt.Errorf("no active backends found for model: %s", modelID)
	}

	// Use Consistent Hashing with Bounded Loads based on user Session/Workflow Key
	ring := NewHashRing()
	for _, b := range backends {
		ring.AddBackend(b)
	}
	ring.Sort()

	// Fetch cache states to prioritize backends that already have our prefix
	cacheStates := r.inferenceReg.GetCacheStates(modelID)

	// Pick target bounding loads
	// This ensures requests from the same session fallback natively to the GPU holding its KV-Cache
	selected := ring.GetBackendWithCacheAffinity(prefixHash, sessionID, cacheStates)

	return &distagentv1.InferenceConfig{
		EndpointUrl: selected.EndpointUrl,
		ModelId:     modelID,
		ApiKey:      "local-no-key-required",
		Temperature: 0,
		PrefixHash:  prefixHash,
	}, nil
}
