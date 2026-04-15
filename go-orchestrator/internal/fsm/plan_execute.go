package fsm

import (
	"errors"
	"fmt"
	"sync"
	"time"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

// PlanExecuteFSM manages the state transitions of a Plan-and-Execute task loop.
type PlanExecuteFSM struct {
	mu         sync.RWMutex
	taskID     string
	state      distagentv1.TaskState
	stepCount  int32
	maxSteps   int32
	history    []string 
	startTime  time.Time
	lastUpdate time.Time
}

// NewPlanExecuteFSM creates a new Plan-and-Execute FSM in the pending state.
func NewPlanExecuteFSM(taskID string, maxSteps int32) *PlanExecuteFSM {
	now := time.Now()
	return &PlanExecuteFSM{
		taskID:     taskID,
		state:      distagentv1.TaskState_TASK_STATE_PENDING,
		stepCount:  0,
		maxSteps:   maxSteps,
		startTime:  now,
		lastUpdate: now,
	}
}

// TaskID returns the task ID.
func (f *PlanExecuteFSM) TaskID() string {
	return f.taskID
}

// State returns the current FSM state.
func (f *PlanExecuteFSM) State() distagentv1.TaskState {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.state
}

// Transition attempts to transition the FSM to the target state.
// It enforces the valid Plan-and-Execute loop state transitions.
func (f *PlanExecuteFSM) Transition(to distagentv1.TaskState) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	from := f.state

	valid := false
	switch from {
	case distagentv1.TaskState_TASK_STATE_PENDING:
		if to == distagentv1.TaskState_TASK_STATE_PLANNING || to == distagentv1.TaskState_TASK_STATE_CANCELLED || to == distagentv1.TaskState_TASK_STATE_FAILED {
			valid = true
		}
	case distagentv1.TaskState_TASK_STATE_PLANNING:
		if to == distagentv1.TaskState_TASK_STATE_PLAN_GENERATED || to == distagentv1.TaskState_TASK_STATE_FAILED || to == distagentv1.TaskState_TASK_STATE_CANCELLED {
			valid = true
		}
	case distagentv1.TaskState_TASK_STATE_PLAN_GENERATED:
		if to == distagentv1.TaskState_TASK_STATE_STEP_EXECUTING || to == distagentv1.TaskState_TASK_STATE_FAILED || to == distagentv1.TaskState_TASK_STATE_CANCELLED || to == distagentv1.TaskState_TASK_STATE_FINAL_ANSWER {
            // Can transition to FINAL_ANSWER if 0 steps.
			valid = true
		}
	case distagentv1.TaskState_TASK_STATE_STEP_EXECUTING:
		if to == distagentv1.TaskState_TASK_STATE_STEP_COMPLETED || to == distagentv1.TaskState_TASK_STATE_REPLANNING || to == distagentv1.TaskState_TASK_STATE_FAILED || to == distagentv1.TaskState_TASK_STATE_CANCELLED {
			valid = true
		}
	case distagentv1.TaskState_TASK_STATE_REPLANNING:
		if to == distagentv1.TaskState_TASK_STATE_PLAN_GENERATED || to == distagentv1.TaskState_TASK_STATE_FAILED || to == distagentv1.TaskState_TASK_STATE_CANCELLED {
			valid = true
		}
	case distagentv1.TaskState_TASK_STATE_STEP_COMPLETED:
		if to == distagentv1.TaskState_TASK_STATE_STEP_EXECUTING || to == distagentv1.TaskState_TASK_STATE_FINAL_ANSWER || to == distagentv1.TaskState_TASK_STATE_FAILED || to == distagentv1.TaskState_TASK_STATE_CANCELLED {
			valid = true
		}
	}

	if !valid {
		return fmt.Errorf("invalid transition from %v to %v", from, to)
	}

	// Step count logic (Micro execution loops)
	if to == distagentv1.TaskState_TASK_STATE_STEP_COMPLETED {
		f.stepCount++
		if f.maxSteps > 0 && f.stepCount > f.maxSteps {
			f.state = distagentv1.TaskState_TASK_STATE_FAILED
			f.lastUpdate = time.Now()
			return errors.New("max execution steps exceeded")
		}
	}

	f.state = to
	f.lastUpdate = time.Now()
	f.history = append(f.history, fmt.Sprintf("%v -> %v", from, to))
	return nil
}

// IsTerminal returns whether the current state is terminal.
func (f *PlanExecuteFSM) IsTerminal() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.state == distagentv1.TaskState_TASK_STATE_FINAL_ANSWER ||
		f.state == distagentv1.TaskState_TASK_STATE_FAILED ||
		f.state == distagentv1.TaskState_TASK_STATE_CANCELLED
}

// StepCount returns current steps taken
func (f *PlanExecuteFSM) StepCount() int32 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.stepCount
}
