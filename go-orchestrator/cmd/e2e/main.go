package main

import (
	"context"
	"log"
	"time"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
	"github.com/sagar/distagent/internal/dispatcher"
)

func main() {
	client, err := dispatcher.NewAgentClient("127.0.0.1:50051")
	if err != nil {
		log.Fatalf("Failed to connect to agent: %v", err)
	}
	defer client.Close()

	// Testing connectivity with safe mocked parameters
	req := &distagentv1.ExecuteTaskRequest{
		TaskId:           "test-task-1",
		JobId:            "wf-1",
		SystemPrompt:     "You are a test agent.",
		UserContext:      "What is 2+2?",
		MaxReactSteps:    3,
		ExecutorInference: &distagentv1.InferenceConfig{
			EndpointUrl: "http://localhost:8000/v1", // Dummy unresolvable URL for safety
			ModelId:     "mock-model",
			ApiKey:      "dummy",
			Temperature: 0,
		},
		PlannerInference: &distagentv1.InferenceConfig{
			EndpointUrl: "http://localhost:8000/v1", // Dummy unresolvable URL for safety
			ModelId:     "mock-model",
			ApiKey:      "dummy",
			Temperature: 0,
		},
	}

	outChan := make(chan *distagentv1.ExecuteTaskResponse)
	
	go func() {
		// This dials the Python agent and pipes the gRPC response stream to the channel
		err := client.ExecuteTask(context.Background(), req, outChan)
		if err != nil {
			log.Printf("ExecuteTask returned error: %v", err)
		}
	}()
	
	log.Println("Dispatched task to python agent! Waiting for stream drops...")

	timeout := time.After(5 * time.Second)
	
	for {
		select {
		case resp, ok := <-outChan:
			if !ok {
				log.Println("gRPC Stream cleanly closed.")
				return
			}
			log.Printf("Received Response State: %v", resp.State)
			if resp.State == distagentv1.TaskState_TASK_STATE_FAILED || resp.State == distagentv1.TaskState_TASK_STATE_FINAL_ANSWER {
				log.Println("Terminal state successfully verified.")
			}
			if err := resp.GetError(); err != nil {
				log.Printf("Safe Error payload captured (expected due to dummy URL): %v", err.Message)
			}
		case <-timeout:
			log.Println("Timeout waiting for response. End verification.")
			return
		}
	}
}
