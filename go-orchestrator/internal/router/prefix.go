package router

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

// ComputePrefixHash computes a deterministic SHA-256 hash of the static prompt prefix (system prompt + tools).
// This hashing logic is duplicated identically in the Python agent worker.
func ComputePrefixHash(systemPrompt string, tools []*distagentv1.ToolDefinition) string {
	h := sha256.New()
	
	// system_prompt.encode("utf-8")
	h.Write([]byte(systemPrompt))
	
	if len(tools) > 0 {
		// Sort tools by name to ensure deterministic hashing regardless of definition order
		sortedTools := make([]*distagentv1.ToolDefinition, len(tools))
		copy(sortedTools, tools)
		sort.Slice(sortedTools, func(i, j int) bool {
			return sortedTools[i].Name < sortedTools[j].Name
		})

		for _, t := range sortedTools {
			h.Write([]byte{0})
			h.Write([]byte(t.Name))
			h.Write([]byte{0})
			h.Write([]byte(t.Description))
			h.Write([]byte{0})
			h.Write([]byte(t.ParametersJsonSchema))
		}
	}
	
	return hex.EncodeToString(h.Sum(nil))
}
