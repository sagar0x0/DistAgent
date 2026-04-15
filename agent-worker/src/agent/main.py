import asyncio
import logging
import grpc
import structlog
from concurrent import futures

from distagent.v1 import agent_pb2_grpc
from agent.server import AgentWorkerServicer
from agent.heartbeat import start_heartbeat_loop
from agent.config import config
import agent.tools.builtins # Automatically registers built-in tools

structlog.configure(
    processors=[
        structlog.processors.TimeStamper(fmt="iso"),
        structlog.processors.JSONRenderer()
    ]
)
logger = structlog.get_logger()

async def serve():
    server = grpc.aio.server(futures.ThreadPoolExecutor(max_workers=10))
    servicer = AgentWorkerServicer()
    agent_pb2_grpc.add_AgentWorkerServiceServicer_to_server(servicer, server)
    
    listen_addr = f"{config.agent_host}:{config.agent_port}"
    server.add_insecure_port(listen_addr)
    
    logger.info("Starting Python Agent Server", address=listen_addr)
    await server.start()
    
    # Start the continuous heartbeat loop dialing the orchestrator in the background
    heartbeat_task = asyncio.create_task(start_heartbeat_loop(servicer))
    
    await server.wait_for_termination()
    heartbeat_task.cancel()

if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO)
    asyncio.run(serve())
