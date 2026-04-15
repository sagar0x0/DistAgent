package stategraph

import (
	"context"
	"encoding/json"
	"log"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

type Supervisor struct {
}

func NewSupervisor() *Supervisor {
	return &Supervisor{}
}

// DecideNextNode returns the next node ID based on Hybrid logic.
func (s *Supervisor) DecideNextNode(ctx context.Context, state *distagentv1.WorkflowState, lastNodeID string) (string, error) {
	// 1. Hardcoded Rules (Deterministic FSM bounds)
	var stateData map[string]interface{}
	if state.SharedContextJson != "" {
		if err := json.Unmarshal([]byte(state.SharedContextJson), &stateData); err == nil {
			if failedSteps, ok := stateData["failed_steps"].(float64); ok && failedSteps > 3 {
				log.Println("[Supervisor] Deterministic catch: Too many failed steps, routing to END")
				return "END", nil
			}
		}
	}

	// 2. Model-Driven Routing (Simulated for MVP)
	// In reality, this constructs a prompt with WorkflowState and asks the LLM.
	
	if lastNodeID == "root_task" {
		log.Println("[Supervisor] LLM decided: Need QA. Routing to qa_node.")
		return "qa_node", nil
	} else if lastNodeID == "qa_node" {
		log.Println("[Supervisor] LLM decided: QA passed. Goal met.")
		return "END", nil
	}
	
	return "END", nil
}
