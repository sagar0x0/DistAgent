import hashlib
from typing import Any, Iterable

def compute_prefix_hash(system_prompt: str, tools: Iterable[Any] = None) -> str:
    """
    Computes a deterministic SHA-256 hash of the static prompt prefix (system prompt + tools).
    This hashing logic is duplicated identically in the Go orchestrator.
    """
    hasher = hashlib.sha256()
    hasher.update(system_prompt.encode("utf-8"))
    
    if tools:
        tool_list = list(tools)
        # Sort by tool name to ensure deterministic hashing regardless of definition order
        tool_list.sort(key=lambda t: t.name)
        for t in tool_list:
            hasher.update(b"\x00")
            hasher.update(t.name.encode("utf-8"))
            hasher.update(b"\x00")
            hasher.update(t.description.encode("utf-8"))
            hasher.update(b"\x00")
            hasher.update(t.parameters_json_schema.encode("utf-8"))
            
    return hasher.hexdigest()
