import json
from typing import Callable, Dict, Any
from distagent.v1 import common_pb2

class ToolRegistry:
    def __init__(self):
        self._tools: Dict[str, Callable] = {}
        self._definitions: Dict[str, common_pb2.ToolDefinition] = {}

    def register(self, name: str, description: str, schema: Dict[str, Any], func: Callable):
        self._tools[name] = func
        self._definitions[name] = common_pb2.ToolDefinition(
            name=name,
            description=description,
            parameters_json_schema=json.dumps(schema)
        )

    def get_definition(self, name: str) -> common_pb2.ToolDefinition:
        return self._definitions.get(name)

    async def invoke(self, name: str, args_json: str) -> str:
        if name not in self._tools:
            raise KeyError(f"Tool {name} not found")
        args = json.loads(args_json)
        func = self._tools[name]
        try:
            import inspect
            if inspect.iscoroutinefunction(func):
                res = await func(**args)
            else:
                res = func(**args)
            return json.dumps(res)
        except Exception as e:
            return json.dumps({"error": str(e)})

default_registry = ToolRegistry()
