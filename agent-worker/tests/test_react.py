import json
import asyncio
import importlib
import pytest
from distagent.v1 import common_pb2, agent_pb2
from agent.tools.registry import ToolRegistry
from agent.react_loop import run_react_task

# Mock InferenceClient
class MockInferenceClient:
    def __init__(self, responses):
        self.responses = responses
        self.call_count = 0

    async def chat_completion_stream(self, config, messages):
        response_text = self.responses[self.call_count]
        self.call_count += 1
        
        # Tokenize roughly
        chunk_size = 5
        for i in range(0, len(response_text), chunk_size):
            yield response_text[i:i+chunk_size]

@pytest.mark.asyncio
async def test_tool_registry():
    reg = ToolRegistry()
    reg.register("test_tool", "desc", {}, lambda: {"success": True})
    
    assert reg.get_definition("test_tool") is not None
    
    res = await reg.invoke("test_tool", "{}")
    assert json.loads(res)["success"] is True

@pytest.mark.asyncio
async def test_react_loop_success():
    req = agent_pb2.ExecuteTaskRequest(
        task_id="task-1",
        system_prompt="You are a mock agent.",
        user_context="What is 2+2?",
        max_react_steps=5
    )
    
    # 1. Provide an action 2. Provide final answer
    mock_responses = [
        "THOUGHT: I need to use the calculator.\nACTION: {\"tool\": \"calculator\", \"args\": {\"expression\": \"2+2\"}}",
        "THOUGHT: I now know the final answer.\nFINAL_ANSWER: 4"
    ]
    
    client = MockInferenceClient(mock_responses)
    
    # Needs the builtins explicitly imported to populate registry
    import agent.tools.builtins
    
    states_seen = []
    
    async for resp in run_react_task(req, client):
        states_seen.append(resp.state)
        if resp.state == common_pb2.TASK_STATE_FINAL_ANSWER:
            assert resp.final_result.answer == "4"
            assert resp.final_result.total_steps == 2
            
    assert common_pb2.TASK_STATE_ACTION_REQUESTED in states_seen
    assert common_pb2.TASK_STATE_OBSERVATION_PROVIDED in states_seen
    assert common_pb2.TASK_STATE_FINAL_ANSWER in states_seen
