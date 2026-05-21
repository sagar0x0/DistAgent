.PHONY: proto proto-go proto-python init clean

proto: proto-go proto-python

proto-go:
	@echo "Generating Go protobuf stubs..."
	@cd proto && buf format -w && buf generate

proto-python:
	@echo "Generating Python protobuf stubs..."
	@mkdir -p agent-worker/gen/distagent/v1
	@touch agent-worker/gen/__init__.py
	@touch agent-worker/gen/distagent/__init__.py
	@touch agent-worker/gen/distagent/v1/__init__.py
	@. agent-worker/.venv/bin/activate && \
	python3 -m grpc_tools.protoc \
		-I./proto \
		--python_out=./agent-worker/gen \
		--grpc_python_out=./agent-worker/gen \
		proto/distagent/v1/*.proto


init:
	@echo "Initializing tools..."
	@go install github.com/bufbuild/buf/cmd/buf@latest

clean:
	@rm -rf go-orchestrator/gen
	@rm -rf agent-worker/gen

bench:
	@echo "Running performance benchmarks..."
	@cd go-orchestrator && go test -bench=. -benchmem -count=3 -timeout=300s ./...

bench-json:
	@echo "Running performance benchmarks with JSON output..."
	@cd go-orchestrator && go test -bench=. -benchmem -count=3 -timeout=300s -json ./... > bench_results.json

bench-profile:
	@echo "Running control plane profile..."
	@cd go-orchestrator && go test -bench=BenchmarkControlPlane -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof ./internal/api/
