from .registry import default_registry

def get_weather(location: str, unit: str = "celsius"):
    """Mock weather tool"""
    return {"location": location, "temperature": 22, "unit": unit, "condition": "Sunny"}

default_registry.register(
    name="get_weather",
    description="Get the current weather in a given location",
    schema={
        "type": "object",
        "properties": {
            "location": {"type": "string", "description": "The city and state, e.g. San Francisco, CA"},
            "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
        },
        "required": ["location"]
    },
    func=get_weather
)

def calculator(expression: str):
    """Mock safe eval"""
    try:
        # DO NOT DO THIS IN PROD, just a mock
        res = eval(expression, {"__builtins__": None}, {})
        return {"result": res}
    except Exception as e:
        return {"error": str(e)}

default_registry.register(
    name="calculator",
    description="Evaluate a mathematical expression. Use for math equations.",
    schema={
        "type": "object",
        "properties": {
            "expression": {"type": "string", "description": "Mathematical expression, e.g. 2 + 2"}
        },
        "required": ["expression"]
    },
    func=calculator
)
