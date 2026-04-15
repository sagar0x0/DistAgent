package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"connectrpc.com/connect"
	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
	"github.com/sagar/distagent/gen/distagent/v1/distagentv1connect"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: go run main.go \"Your prompt here\"")
	}
	prompt := os.Args[1]

	client := distagentv1connect.NewOrchestratorServiceClient(
		http.DefaultClient,
		"http://localhost:8080",
	)

	req := connect.NewRequest(&distagentv1.SubmitJobRequest{
		SessionId:  "session-123",
		UserPrompt: prompt,
	})

	log.Printf("Submitting job to distagent-orchestrator: %q", prompt)

	stream, err := client.SubmitJob(context.Background(), req)
	if err != nil {
		log.Fatalf("SubmitJob failed: %v", err)
	}

	for stream.Receive() {
		resp := stream.Msg()
		switch ev := resp.Event.(type) {
		case *distagentv1.SubmitJobResponse_Status:
			fmt.Printf("[STATUS] %s: %s\n", ev.Status.State, ev.Status.Message)
		case *distagentv1.SubmitJobResponse_Thinking:
			fmt.Printf("[PLANNING] Step %d: %s\n", ev.Thinking.StepNumber, ev.Thinking.ThoughtText)
		case *distagentv1.SubmitJobResponse_Action:
			fmt.Printf("[EXECUTION] Invoking '%s' with %s\n", ev.Action.ToolName, ev.Action.ArgumentsJson)
		case *distagentv1.SubmitJobResponse_FinalAnswer:
			fmt.Printf("\n[FINAL RESULT]\n%s\n", ev.FinalAnswer.AnswerText)
		}
	}

	if err := stream.Err(); err != nil {
		log.Fatalf("Stream error: %v", err)
	}
}
