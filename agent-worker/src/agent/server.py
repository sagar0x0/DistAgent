import asyncio
from distagent.v1 import agent_pb2, agent_pb2_grpc, common_pb2
import structlog
from typing import AsyncGenerator

from .plan_execute_loop import run_plan_execute_task
from .llm_client import InferenceClient

logger = structlog.get_logger()

class AgentWorkerServicer(agent_pb2_grpc.AgentWorkerServiceServicer):
    def __init__(self):
        super().__init__()
        self.llm_client = InferenceClient()
        self.active_tasks = 0

    async def ExecuteTask(
        self, request: agent_pb2.ExecuteTaskRequest, context
    ) -> AsyncGenerator[agent_pb2.ExecuteTaskResponse, None]:
        
        self.active_tasks += 1
        try:
            async for response in run_plan_execute_task(request, self.llm_client):
                yield response
        except Exception as e:
            logger.error("task_execution_failed", error=str(e), task_id=request.task_id)
            yield agent_pb2.ExecuteTaskResponse(
                task_id=request.task_id,
                state=common_pb2.TASK_STATE_FAILED,
                error=agent_pb2.ErrorPayload(code="INTERNAL_ERROR", message=str(e), retryable=False)
            )
        finally:
            self.active_tasks -= 1

    async def Heartbeat(self, request_iterator, context):
        # We handle bidi streaming if needed, but usually Python just sends and receives Acks
        async for hb in request_iterator:
            logger.debug("received_heartbeat_request", agent=hb.agent_id)
            # The orchestrator is sending us something? Wait, orchestrator listens to heartbeats.
            # Actually, agent initiates the stream to orchestrator. Wait.
            # In agent.proto: rpc Heartbeat(stream HeartbeatRequest) returns (stream HeartbeatResponse);
            # The Agent IS the gRPC Server per the architecture. 
            # Wait, orchestrator calls `Heartbeat` RPC on the agent?
            # Yes, if Agent is the server, Orchestrator connects to Agent to scrape heartbeat, OR agent connects to Orchestrator.
            # Usually, workers register to orchestrators. But since Orchestrator is driving and dialing, 
            # let's assume Orchestrator dials Agent and opens Heartbeat stream.
            yield agent_pb2.HeartbeatResponse(accepted=True, message="Agent is alive")

    async def Drain(self, request: common_pb2.DrainRequest, context):
        logger.info("drain_requested")
        return common_pb2.DrainResponse(acknowledged=True, tasks_remaining=self.active_tasks)
