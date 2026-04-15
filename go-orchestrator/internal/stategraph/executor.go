package stategraph

import (
	"context"
	"fmt"
	"log"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

// StateGraphExecutor runs the adaptive state graph.
type StateGraphExecutor struct {
	supervisor *Supervisor
}

// NewStateGraphExecutor creates an executor.
func NewStateGraphExecutor(supervisor *Supervisor) *StateGraphExecutor {
	return &StateGraphExecutor{
		supervisor: supervisor,
	}
}

// Execute runs the state graph starting at initialNodeID.
func (e *StateGraphExecutor) Execute(
	ctx context.Context,
	initialNodeID string,
	initialState *distagentv1.WorkflowState,
	dispatchFn func(ctx context.Context, nodeID string, state *distagentv1.WorkflowState) (*distagentv1.WorkflowState, error),
) error {
	currentState := initialState
	currentNodeID := initialNodeID

	maxNodesExecution := 20
	iterations := 0

	for currentNodeID != "END" {
		if iterations >= maxNodesExecution {
			return fmt.Errorf("state graph exceeded maximum allowed iterations (%d)", maxNodesExecution)
		}
		iterations++

		log.Printf("[StateGraph] Executing Node: %s", currentNodeID)
		currentState.ExecutionTrail = append(currentState.ExecutionTrail, currentNodeID)

		// 1. Dispatch to Agent
		var err error
		if currentNodeID != "supervisor" {
			currentState, err = dispatchFn(ctx, currentNodeID, currentState)
			if err != nil {
				return fmt.Errorf("node %s failed: %w", currentNodeID, err)
			}
		}

		// 2. Supervisor routing (Hybrid approach)
		nextNodeID, err := e.supervisor.DecideNextNode(ctx, currentState, currentNodeID)
		if err != nil {
			return fmt.Errorf("supervisor routing failed: %w", err)
		}

		log.Printf("[StateGraph] Supervisor routed %s -> %s", currentNodeID, nextNodeID)
		currentNodeID = nextNodeID
	}

	log.Printf("[StateGraph] Workflow complete.")
	return nil
}
