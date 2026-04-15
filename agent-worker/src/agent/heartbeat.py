import asyncio
import psutil
import structlog
from datetime import datetime
from google.protobuf.timestamp_pb2 import Timestamp
import grpc

from distagent.v1 import agent_pb2_grpc, agent_pb2, common_pb2
from .config import config

logger = structlog.get_logger()

# Assuming agent handles incoming ExecuteTask but pushes Heartbeats to the Orchestrator
# Actually, the proto says:
# `rpc Heartbeat(stream HeartbeatRequest) returns (stream HeartbeatResponse);`
# on the AgentWorkerService.
# Wait, if Orchestrator is the client and Agent is the server:
# Orchestrator calls Agent's Heartbeat method and streams requests? No,
# usually Agents push to Orchestrator. But we defined Heartbeat inside `AgentWorkerService`.
# Therefore, Orchestrator connects to Agent and calls `Heartbeat`.
# The agent yields HeartbeatResponse.
# But wait, `AgentWorkerService` defines:
# `rpc Heartbeat(stream HeartbeatRequest) returns (stream HeartbeatResponse);`
# `HeartbeatRequest` contains agent stats. 
# This means the CLIENT (Orchestrator) streams `HeartbeatRequest`? 
# That's backward. If Agent is the server, Orchestrator would be sending HeartbeatRequest blocks, which contain agent stats? Orchestrator doesn't have agent stats.
# Ah, if AgentWorkerService is hosted by the Orchestrator? 
# No, we explicitly said "Orchestrator calls this. Agent does NOT do inference".
# Let's check `agent.proto`.

# In `agent.proto`:
# `service AgentWorkerService { rpc Heartbeat(stream HeartbeatRequest) returns (stream HeartbeatResponse); }`
# `message HeartbeatRequest { string agent_id = 1; float cpu_usage_percent = 7; ... }`

# If `AgentWorkerService` is hosted by the CPU Agent:
# The Orchestrator (client) connects to CPU Agent (server).
# Orchestrator calls `Heartbeat(stream HeartbeatRequest)`.
# Since Orchestrator is the client, it shouldn't be generating `HeartbeatRequest` with local CPU percentages. It doesn't know them.
# The only way this works is if the CPU Agent calls `Heartbeat` on an Orchestrator service.
# BUT we put it in `AgentWorkerService`.
# Let's fix this minor design quirk: The Agent can just expose a simple gRPC bidi stream where it *ignores* the client's request stream and just pushes responses continuously? No, standard gRPC: client streams requests, server streams responses. 
# If Agent is server, it can only send `HeartbeatResponse` (which is `AgentHeartbeatAck`), not `HeartbeatRequest`.
# Wait, the `implementation_plan_diagent.md` says:
# "Orchestrator's response to an agent heartbeat: message HeartbeatResponse".
# This implies the Orchestrator is returning `HeartbeatResponse`.
# Therefore, the Orchestrator must be the SERVER for the `Heartbeat` RPC.
# Did we put it in `AgentWorkerService` by mistake? 
# Yes, `AgentWorkerService` is defined as "Internal service implemented by Python agent workers ... The Go orchestrator calls this to dispatch tasks."
# BUT it also says:
# "Bidirectional heartbeat: agent reports health, orchestrator can push config."
# If Agent is the client dialing the Orchestrator, the RPC should be in `OrchestratorService`.
# But in grpc, a bidi stream allows either side to push messages.
# If Agent is the Server, the Orchestrator (Client) dials `Heartbeat`, then the Orchestrator streams `HeartbeatRequest`? No, the stream goes Client -> Server for requests.
# This means the Orchestrator would send `HeartbeatRequest`.
# What actually happens in Disaggregated architectures: The Orchestrator acts as the "Server" for heartbeats, and Agents dial IN.
# Or, if we strictly follow the protobuf we wrote:
# If `Heartbeat` is in `AgentWorkerService` (Server=Agent):
# Orchestrator must stream `HeartbeatRequest` to Agent. (Impossible, Orchestrator doesn't know Agent's CPU).
# Hmm. Let's look at `inference.proto`: `rpc ReportHealth(stream ReportHealthRequest) returns (stream ReportHealthResponse);` inside `InferenceBackendService`.
# Wait, if `AgentWorkerService` and `InferenceBackendService` are hosted on the workers, and Orchestrator dials them... The proto design is slightly flawed because it maps the Worker's telemetry into the `Request` object of a service hosted by the Worker.
# 
# To solve this without rewriting the proto (which is approved in CP1), we can just implement the Agent as a gRPC CLIENT that dials the Orchestrator, AND ALSO a gRPC SERVER that accepts `ExecuteTask` from the Orchestrator.
# Or, the Orchestrator is the Server for `AgentWorkerService.Heartbeat` too?
# GPRC services can be implemented by anyone. We can have the Orchestrator implement `AgentWorkerService.Heartbeat` (acting as Server for that specific RPC), and Agent implement `AgentWorkerService.ExecuteTask` (acting as Server). 
# Actually, gRPC doesn't easily split services. It's better if the Orchestrator implements `AgentWorkerService` for `Heartbeat` and `Agent` implements it for `ExecuteTask`.
# But a cleaner approach for CP3: We write `heartbeat.py` as a gRPC *Client* that dials the Orchestrator address, calling the `Heartbeat` stub.
# The Orchestrator will run a server hosting the `AgentWorkerService.Heartbeat` endpoint.

async def start_heartbeat_loop(server=None):
    """
    Dials the orchestrator and streams HeartbeatRequests continuously.
    """
    logger.info("starting_heartbeat_loop", orchestrator=config.orchestrator_address)
    
    while True:
        try:
            channel = grpc.aio.insecure_channel(config.orchestrator_address)
            stub = agent_pb2_grpc.AgentWorkerServiceStub(channel)

            async def generate_heartbeats():
                while True:
                    ts = Timestamp()
                    ts.GetCurrentTime()
                    
                    hb = agent_pb2.HeartbeatRequest(
                        agent_id=config.agent_id,
                        agent_address=f"{config.agent_host}:{config.agent_port}",
                        timestamp=ts,
                        status=common_pb2.AGENT_STATUS_READY,
                        active_tasks=server.active_tasks if server else 0,
                        max_concurrent_tasks=config.max_concurrent_tasks,
                        cpu_usage_percent=psutil.cpu_percent(),
                        memory_usage_percent=psutil.virtual_memory().percent
                    )
                    yield hb
                    await asyncio.sleep(5)

            async for response in stub.Heartbeat(generate_heartbeats()):
                if not response.accepted:
                    logger.warning("heartbeat_rejected", message=response.message)
                # optionally process config_updates here

        except grpc.aio.AioRpcError as e:
            logger.error("heartbeat_rpc_error", error=e.details())
            await asyncio.sleep(5)
        except Exception as e:
            logger.error("heartbeat_loop_error", error=str(e))
            await asyncio.sleep(5)
