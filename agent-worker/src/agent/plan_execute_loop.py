import json
import asyncio
from typing import AsyncGenerator, List, Dict, Any
import structlog

from distagent.v1 import agent_pb2, common_pb2
from .llm_client import InferenceClient
from .tools.registry import default_registry
from .prefix_hash import compute_prefix_hash

logger = structlog.get_logger()

def build_planner_system_prompt(system_prompt: str) -> str:
    return f"""{system_prompt}

You are the Master Planner in a Plan-and-Execute agent architecture.
Your job is to analyze the user's request and the conversation history, and output a strictly formatted JSON array of execution steps required to solve the task.

Requirements:
- Output ONLY a valid JSON array of strings. Do not output anything else.
- Each string should be a clear, actionable instruction for an execution agent.
- Do NOT try to solve the problem yourself, just break it down into steps.
- If no steps are needed because the answer is already known, output a single step explaining what final answer to formulate.

Example output:
[
  "Search the web for the latest news on SpaceX.",
  "Extract the names of the rockets mentioned.",
  "Calculate the sum of their payload capacities."
]
"""

def build_executor_system_prompt(system_prompt: str) -> str:
    return f"""{system_prompt}

You are the Execution Agent. You are given a specific "Step Goal" to complete.
Think step-by-step. Use your native tools to execute the steps securely.
Once you have retrieved the core information or finished your logic, formulate the final step insight in your response.
"""

async def run_plan_execute_task(
    request: agent_pb2.ExecuteTaskRequest,
    llm_client: InferenceClient
) -> AsyncGenerator[agent_pb2.ExecuteTaskResponse, None]:
    
    logger.info("starting_plan_execute_loop", task_id=request.task_id)

    # ---------------------------------------------------------
    # PHASE 1: PLANNING
    # ---------------------------------------------------------
    yield agent_pb2.ExecuteTaskResponse(
        task_id=request.task_id,
        state=common_pb2.TASK_STATE_PLANNING
    )
    
    planner_sys_content = build_planner_system_prompt(request.system_prompt)
    planner_prefix_hash = compute_prefix_hash(planner_sys_content)
    logger.info("computed_planner_prefix_hash", hash=planner_prefix_hash)

    planner_messages = [{"role": "system", "content": planner_sys_content}]
    for msg in request.history:
        planner_messages.append({"role": msg.role, "content": msg.content})
    planner_messages.append({"role": "user", "content": request.user_context})

    planner_response = ""
    try:
        async for chunk in llm_client.chat_completion_stream(request.planner_inference, planner_messages, prefix_hash=planner_prefix_hash):
            if chunk.content:
                planner_response += chunk.content
                yield agent_pb2.ExecuteTaskResponse(
                    task_id=request.task_id,
                    state=common_pb2.TASK_STATE_PLANNING,
                    token=agent_pb2.TokenPayload(delta=chunk.content)
                )
    except Exception as e:
        yield agent_pb2.ExecuteTaskResponse(
            task_id=request.task_id,
            state=common_pb2.TASK_STATE_FAILED,
            error=agent_pb2.ErrorPayload(code="PLANNER_ERROR", message=str(e), retryable=True)
        )
        return

    # Extract JSON Array of steps
    steps = []
    try:
        # Find json array block
        start_idx = planner_response.find('[')
        end_idx = planner_response.rfind(']') + 1
        if start_idx == -1 or end_idx == 0:
            raise ValueError("No steps array found in planner output")
        steps = json.loads(planner_response[start_idx:end_idx])
        if not isinstance(steps, list):
            raise ValueError("Parsed JSON is not a list")
    except Exception as e:
        # Re-plan fallback (rudimentary implementation: just fail and let supervisor retry)
        yield agent_pb2.ExecuteTaskResponse(
            task_id=request.task_id,
            state=common_pb2.TASK_STATE_FAILED,
            error=agent_pb2.ErrorPayload(code="PLAN_PARSE_ERROR", message=str(e), retryable=True)
        )
        return

    for i, step_desc in enumerate(steps):
        yield agent_pb2.ExecuteTaskResponse(
            task_id=request.task_id,
            state=common_pb2.TASK_STATE_PLAN_GENERATED,
            thought=agent_pb2.ThoughtPayload(text=step_desc, step=i+1)
        )

    yield agent_pb2.ExecuteTaskResponse(
        task_id=request.task_id,
        state=common_pb2.TASK_STATE_PLAN_GENERATED,
        plan=agent_pb2.PlanPayload(steps=steps)
    )

    # ---------------------------------------------------------
    # PHASE 2: EXECUTION
    # ---------------------------------------------------------
    executor_system = build_executor_system_prompt(request.system_prompt)
    executor_prefix_hash = compute_prefix_hash(executor_system, request.allowed_tools)
    logger.info("computed_executor_prefix_hash", hash=executor_prefix_hash)
    
    openai_tools = []
    if request.allowed_tools:
        for t in request.allowed_tools:
            openai_tools.append({
                "type": "function",
                "function": {
                    "name": t.name,
                    "description": t.description,
                    "parameters": json.loads(t.parameters_json_schema)
                }
            })
    
    # We maintain a running memory for the executor, starting with the original context.
    executor_memory = []
    for msg in request.history:
        executor_memory.append({"role": msg.role, "content": msg.content})
    executor_memory.append({"role": "user", "content": request.user_context})
    executor_memory.append({"role": "assistant", "content": f"I have formulated a plan with {len(steps)} steps. I will now execute it."})

    final_state_json_diff = {} # Could track modified state variables
    step_results = []
    
    for i, step_desc in enumerate(steps):
        yield agent_pb2.ExecuteTaskResponse(
            task_id=request.task_id,
            state=common_pb2.TASK_STATE_STEP_EXECUTING,
            step=agent_pb2.StepPayload(step_index=i+1, step_description=step_desc, step_result="")
        )
        
        step_prompt = f"STEP {i+1}/{len(steps)} GOAL: {step_desc}\nExecute this step using your tools if necessary. Reply with FINAL_RESULT when done."
        executor_messages = [{"role": "system", "content": executor_system}] + executor_memory + [{"role": "user", "content": step_prompt}]
        
        # Micro iteration shield for tool failures per step
        max_micro_steps = 5
        micro_step = 0
        step_completed = False
        step_result_text = ""
        
        while micro_step < max_micro_steps and not step_completed:
            micro_step += 1
            
            # Call Executor LLM
            llm_response_content = ""
            tool_calls = {}
            
            try:
                async for chunk in llm_client.chat_completion_stream(request.executor_inference, executor_messages, openai_tools, prefix_hash=executor_prefix_hash):
                    if chunk.content:
                        llm_response_content += chunk.content
                        yield agent_pb2.ExecuteTaskResponse(
                            task_id=request.task_id,
                            state=common_pb2.TASK_STATE_STEP_EXECUTING,
                            token=agent_pb2.TokenPayload(delta=chunk.content)
                        )
                    if getattr(chunk, 'tool_calls', None):
                        for tc in chunk.tool_calls:
                            idx = tc.index
                            if idx not in tool_calls:
                                tool_calls[idx] = {
                                    "id": getattr(tc, "id", f"call_{idx}"),
                                    "type": "function",
                                    "function": {"name": getattr(tc.function, "name", ""), "arguments": ""}
                                }
                            if tc.function and getattr(tc.function, "arguments", None):
                                tool_calls[idx]["function"]["arguments"] += tc.function.arguments
            except Exception as e:
                yield agent_pb2.ExecuteTaskResponse(
                    task_id=request.task_id,
                    state=common_pb2.TASK_STATE_FAILED,
                    error=agent_pb2.ErrorPayload(code="EXECUTOR_ERROR", message=str(e), retryable=True)
                )
                return

            if tool_calls:
                # Add the assistant's tool-call response to memory
                assistant_msg = {
                    "role": "assistant",
                    "content": llm_response_content or None,
                    "tool_calls": []
                }
                for idx, tc in tool_calls.items():
                    assistant_msg["tool_calls"].append(tc)
                executor_messages.append(assistant_msg)
                
                # Execute each tool
                for idx, tc in tool_calls.items():
                    tool_name = tc["function"]["name"]
                    tool_args = tc["function"]["arguments"]
                    tool_call_id = tc["id"]
                    
                    yield agent_pb2.ExecuteTaskResponse(
                        task_id=request.task_id,
                        state=common_pb2.TASK_STATE_STEP_EXECUTING,
                        action=agent_pb2.ActionPayload(
                            tool_name=tool_name,
                            tool_call_id=tool_call_id,
                            arguments_json=tool_args,
                            step=i+1
                        )
                    )
                    
                    try:
                        args_dict = json.loads(tool_args)
                        obs_res = await default_registry.invoke(tool_name, json.dumps(args_dict))
                        executor_messages.append({"role": "tool", "tool_call_id": tool_call_id, "content": str(obs_res)})
                    except Exception as e:
                        obs_err = f"Error executing {tool_name}: {str(e)}"
                        executor_messages.append({"role": "tool", "tool_call_id": tool_call_id, "content": str(obs_err)})
            else:
                # No tools called. This means the step is complete.
                executor_messages.append({"role": "assistant", "content": llm_response_content})
                # Clean up the FINAL_RESULT wrapper that the LLM tends to add
                step_result_text = llm_response_content.replace("FINAL_RESULT:", "").replace("FINAL_RESULT", "").strip()
                step_completed = True
                break
        
        if not step_completed:
            # We failed to complete the step. Send REPLANNING event
            yield agent_pb2.ExecuteTaskResponse(
                task_id=request.task_id,
                state=common_pb2.TASK_STATE_REPLANNING
            )
            # In a full robust implementation, we would jump back up to the Planner phase here.
            # For this MVP, we abort and fail up to the Go Supervisor.
            yield agent_pb2.ExecuteTaskResponse(
                task_id=request.task_id,
                state=common_pb2.TASK_STATE_FAILED,
                error=agent_pb2.ErrorPayload(code="MICRO_STEPS_EXCEEDED", message=f"Failed to complete step {i+1} after {max_micro_steps} tool attempts.", retryable=True)
            )
            return

        # Record step result to global execution memory
        execution_summary = f"Step {i+1} completed: {step_result_text}"
        step_results.append(execution_summary)
        executor_memory.append({"role": "assistant", "content": execution_summary})
        
        yield agent_pb2.ExecuteTaskResponse(
            task_id=request.task_id,
            state=common_pb2.TASK_STATE_STEP_COMPLETED,
            step=agent_pb2.StepPayload(step_index=i+1, step_description=step_desc, step_result=step_result_text)
        )

    # ---------------------------------------------------------
    # FINAL REDUCTION
    # ---------------------------------------------------------
    # The final answer is just the output of the very last step, rather than 
    # a raw dump of every step's internal reasoning.
    final_answer = step_results[-1].split("completed: ", 1)[-1] if step_results else "No result generated."
    
    # We mutate the global state (in a real system, we might ask the LLM to write a JSON patch)
    old_state = {}
    if request.current_state.shared_context_json:
        try:
            old_state = json.loads(request.current_state.shared_context_json)
        except:
            pass
    old_state["last_agent_output"] = final_answer
    state_update_json = json.dumps(old_state)

    logger.info(
        "plan_execute_complete",
        task_id=request.task_id,
        total_steps=len(steps),
        planner_prefix_hash=planner_prefix_hash,
        executor_prefix_hash=executor_prefix_hash,
    )

    yield agent_pb2.ExecuteTaskResponse(
        task_id=request.task_id,
        state=common_pb2.TASK_STATE_FINAL_ANSWER,
        final_result=agent_pb2.FinalPayload(
            answer=final_answer, 
            total_steps=len(steps),
            state_update_json=state_update_json
        )
    )
