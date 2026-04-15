"""
Tests for agent-worker/src/agent/prefix_hash.py

Covers:
  1. Determinism — same inputs always produce the same digest
  2. Order invariance — tool registration order doesn't affect the hash
  3. Empty / None tools edge case
  4. Different prompts produce different hashes (collision sanity check)
  5. Cross-language golden digest — must match the Go ComputePrefixHash output
"""

import hashlib
import pytest
import sys
import os

# Ensure the src/ directory is importable when pytest is run from agent-worker/
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from agent.prefix_hash import compute_prefix_hash


# ---------------------------------------------------------------------------
# Tiny stub that mimics a protobuf ToolDefinition for testing purposes
# (avoids needing the generated proto stubs to be importable in pytest)
# ---------------------------------------------------------------------------
class _Tool:
    def __init__(self, name: str, description: str, parameters_json_schema: str):
        self.name = name
        self.description = description
        self.parameters_json_schema = parameters_json_schema


def _make_tools():
    return [
        _Tool("calculator",  "Evaluates math expressions", '{"type":"object"}'),
        _Tool("get_weather", "Returns weather for a city",  '{"type":"object"}'),
    ]


# ---------------------------------------------------------------------------
# The golden digest is the output of running compute_prefix_hash() with
# system_prompt = "You are an AI assistant." and the two tools above.
#
# The Go side (TestComputePrefixHash_CrossLanguageGolden in prefix_test.go)
# must produce the same value. If either implementation drifts, one of the
# two golden tests will fail.
# ---------------------------------------------------------------------------
GOLDEN_DIGEST = "f99dbcd26f78191a4c3e29864753936ba81055fc271d7a5fef6bb4f4d0cb0a91"
GOLDEN_PROMPT = "You are an AI assistant."


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

def test_deterministic():
    """Same inputs always produce the same SHA-256 digest."""
    tools = _make_tools()
    h1 = compute_prefix_hash(GOLDEN_PROMPT, tools)
    h2 = compute_prefix_hash(GOLDEN_PROMPT, tools)
    assert h1 == h2, f"non-deterministic: {h1} vs {h2}"


def test_order_invariant():
    """Tool list order must not affect the output digest."""
    prompt = GOLDEN_PROMPT
    tools_asc  = _make_tools()                    # calculator, get_weather
    tools_desc = list(reversed(_make_tools()))    # get_weather, calculator

    h_asc  = compute_prefix_hash(prompt, tools_asc)
    h_desc = compute_prefix_hash(prompt, tools_desc)

    assert h_asc == h_desc, (
        f"hash changed with reversed tool order:\n  asc={h_asc}\n desc={h_desc}"
    )


def test_none_tools_equals_empty_list():
    """None and an empty list should produce identical digests."""
    h_none  = compute_prefix_hash(GOLDEN_PROMPT, None)
    h_empty = compute_prefix_hash(GOLDEN_PROMPT, [])
    assert h_none == h_empty, f"None vs [] mismatch: {h_none} vs {h_empty}"


def test_no_tools_returns_non_empty_digest():
    """Even with no tools the function must return a valid hex digest."""
    digest = compute_prefix_hash("System prompt only", None)
    assert len(digest) == 64, f"expected 64-char hex digest, got {len(digest)}: {digest}"


def test_different_prompts_produce_different_digests():
    """Collision sanity check: different system prompts → different hashes."""
    tools = _make_tools()
    h1 = compute_prefix_hash("Prompt A", tools)
    h2 = compute_prefix_hash("Prompt B", tools)
    assert h1 != h2, "different prompts produced the same digest (collision)"


def test_different_tools_produce_different_digests():
    """Adding a new tool must change the digest."""
    prompt = GOLDEN_PROMPT
    tools_2 = _make_tools()
    tools_3 = _make_tools() + [_Tool("search", "Web search", '{"type":"object"}')]

    h2 = compute_prefix_hash(prompt, tools_2)
    h3 = compute_prefix_hash(prompt, tools_3)
    assert h2 != h3, "adding a tool did not change the prefix hash"


def test_cross_language_golden_digest():
    """
    THE CRITICAL TEST.

    The digest produced by compute_prefix_hash() must be byte-for-byte identical
    to the digest produced by the Go ComputePrefixHash() for the same inputs.

    If this test fails, the Python and Go implementations have drifted and
    cross-service routing decisions will be inconsistent.

    The Go golden test lives in:
      go-orchestrator/internal/router/prefix_test.go
      → TestComputePrefixHash_CrossLanguageGolden
    """
    tools = _make_tools()
    got = compute_prefix_hash(GOLDEN_PROMPT, tools)

    assert got == GOLDEN_DIGEST, (
        f"Cross-language golden digest mismatch!\n"
        f"  Python produced : {got}\n"
        f"  Expected (Go)   : {GOLDEN_DIGEST}\n\n"
        "Check that both implementations use the same byte layout:\n"
        "  system_prompt || \\x00 || name || \\x00 || description || \\x00 || schema\n"
        "  (tools sorted alphabetically by name)"
    )


def test_empty_prompt_no_tools():
    """Edge case: empty string prompt with no tools should still return a digest."""
    digest = compute_prefix_hash("", None)
    # SHA-256 of empty bytes = e3b0c44298fc1c149afb...
    expected = hashlib.sha256(b"").hexdigest()
    assert digest == expected, f"empty prompt hash wrong: {digest}"
