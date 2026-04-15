import json
from typing import AsyncGenerator, Dict, Any, List
from openai import AsyncOpenAI
import structlog

from distagent.v1 import common_pb2

logger = structlog.get_logger()

class InferenceClient:
    """Calls any OpenAI-compatible endpoint. Works with SGLang, LiteLLM, OpenAI directly."""

    async def chat_completion_stream(
        self, inference_config: common_pb2.InferenceConfig, messages: List[Dict[str, Any]], tools: List[Dict[str, Any]] = None, prefix_hash: str = None
    ) -> AsyncGenerator[Any, None]:
        client = AsyncOpenAI(
            base_url=inference_config.endpoint_url,
            api_key=inference_config.api_key or "not-needed",
        )
        try:
            kwargs = {
                "model": inference_config.model_id,
                "messages": messages,
                "temperature": inference_config.temperature,
                "max_tokens": inference_config.max_tokens or 1024,
                "stream": True,
            }
            if tools:
                kwargs["tools"] = tools
            if prefix_hash:
                kwargs["extra_headers"] = {"X-Prefix-Hash": prefix_hash}

            stream = await client.chat.completions.create(**kwargs)
            async for chunk in stream:
                if len(chunk.choices) > 0:
                    yield chunk.choices[0].delta
        except Exception as e:
            logger.error("inference_stream_failed", error=str(e), model=inference_config.model_id)
            raise e
