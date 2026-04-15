package router

import (
	"testing"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

// ---------------------------------------------------------------------------
// Cross-language golden digest
//
// This constant is the SHA-256 digest computed by running the *Python*
// compute_prefix_hash() with the exact same inputs:
//
//   system_prompt = "You are an AI assistant."
//   tools         = [
//     ToolDefinition{Name:"calculator",  Description:"Evaluates math expressions", ParametersJsonSchema:"{\"type\":\"object\"}"},
//     ToolDefinition{Name:"get_weather", Description:"Returns weather for a city",  ParametersJsonSchema:"{\"type\":\"object\"}"},
//   ]
//
// Run from agent-worker/:
//   python -c "
//   import hashlib, sys
//   h = hashlib.sha256()
//   h.update('You are an AI assistant.'.encode())
//   for name, desc, schema in sorted([
//       ('calculator',  'Evaluates math expressions', '{\"type\":\"object\"}'),
//       ('get_weather', 'Returns weather for a city',  '{\"type\":\"object\"}'),
//   ], key=lambda x: x[0]):
//       h.update(b'\x00'); h.update(name.encode())
//       h.update(b'\x00'); h.update(desc.encode())
//       h.update(b'\x00'); h.update(schema.encode())
//   print(h.hexdigest())
//   "
// ---------------------------------------------------------------------------
const crossLangGoldenDigest = "f99dbcd26f78191a4c3e29864753936ba81055fc271d7a5fef6bb4f4d0cb0a91"

func makeTools() []*distagentv1.ToolDefinition {
	return []*distagentv1.ToolDefinition{
		{Name: "calculator", Description: "Evaluates math expressions", ParametersJsonSchema: `{"type":"object"}`},
		{Name: "get_weather", Description: "Returns weather for a city", ParametersJsonSchema: `{"type":"object"}`},
	}
}

// TestComputePrefixHash_Deterministic verifies the same inputs always produce the same hash.
func TestComputePrefixHash_Deterministic(t *testing.T) {
	prompt := "You are an AI assistant."
	tools := makeTools()

	h1 := ComputePrefixHash(prompt, tools)
	h2 := ComputePrefixHash(prompt, tools)

	if h1 != h2 {
		t.Errorf("hash not deterministic: %s != %s", h1, h2)
	}
}

// TestComputePrefixHash_OrderInvariant verifies that tool registration order does not affect the hash.
func TestComputePrefixHash_OrderInvariant(t *testing.T) {
	prompt := "You are an AI assistant."

	// Tools provided in reverse alphabetical order
	reversed := []*distagentv1.ToolDefinition{
		{Name: "get_weather", Description: "Returns weather for a city", ParametersJsonSchema: `{"type":"object"}`},
		{Name: "calculator", Description: "Evaluates math expressions", ParametersJsonSchema: `{"type":"object"}`},
	}
	sorted := makeTools() // already alphabetical

	h1 := ComputePrefixHash(prompt, sorted)
	h2 := ComputePrefixHash(prompt, reversed)

	if h1 != h2 {
		t.Errorf("hash is not order-invariant: sorted=%s reversed=%s", h1, h2)
	}
}

// TestComputePrefixHash_EmptyTools verifies no-tools case doesn't panic and is stable.
func TestComputePrefixHash_EmptyTools(t *testing.T) {
	prompt := "You are an AI assistant."

	h1 := ComputePrefixHash(prompt, nil)
	h2 := ComputePrefixHash(prompt, []*distagentv1.ToolDefinition{})

	if h1 != h2 {
		t.Errorf("nil vs empty tools produced different hashes: %s vs %s", h1, h2)
	}
	if h1 == "" {
		t.Error("expected non-empty hash for no-tools case")
	}
}

// TestComputePrefixHash_DifferentPromptsProduceDifferentHashes is a sanity check.
func TestComputePrefixHash_DifferentPromptsProduceDifferentHashes(t *testing.T) {
	tools := makeTools()
	h1 := ComputePrefixHash("System prompt A", tools)
	h2 := ComputePrefixHash("System prompt B", tools)

	if h1 == h2 {
		t.Error("different system prompts produced the same hash (collision)")
	}
}

// TestComputePrefixHash_CrossLanguageGolden is the critical cross-language determinism test.
// The expected digest was computed by running the Python compute_prefix_hash() on
// the same inputs (see the constant definition above for the exact Python command).
// If this test fails, the Go and Python implementations have drifted apart and
// routing decisions will be inconsistent between the orchestrator and the agent worker.
func TestComputePrefixHash_CrossLanguageGolden(t *testing.T) {
	prompt := "You are an AI assistant."
	tools := makeTools()

	got := ComputePrefixHash(prompt, tools)

	if got != crossLangGoldenDigest {
		t.Errorf(
			"cross-language golden digest mismatch\n  Go produced : %s\n  Python gave  : %s\n\nVerify the Python implementation matches the byte layout in prefix.go",
			got, crossLangGoldenDigest,
		)
	}
}
