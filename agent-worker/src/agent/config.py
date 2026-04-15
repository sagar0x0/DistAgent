from pydantic_settings import BaseSettings, SettingsConfigDict

class AgentConfig(BaseSettings):
    agent_id: str = "agent-local"
    agent_host: str = "0.0.0.0"
    agent_port: int = 50051
    orchestrator_address: str = "127.0.0.1:8081"
    max_concurrent_tasks: int = 10
    
    model_config = SettingsConfigDict(env_prefix="DISTAGENT_")

config = AgentConfig()
