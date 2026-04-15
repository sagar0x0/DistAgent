FROM python:3.12-slim

WORKDIR /app
# Copy shared protobuf stubs
COPY gen/ /app/gen/
COPY agent-worker/ /app/agent-worker/

WORKDIR /app/agent-worker
RUN pip install --no-cache-dir -r requirements.txt

# Ensure Python finds the proto generated stubs and our internal source logic natively
ENV PYTHONPATH="/app/agent-worker/src:/app/gen"

EXPOSE 50051

CMD ["python", "src/agent/main.py"]
