package fsm

import (
    "testing"
    
    distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

func TestPlanExecuteFSM_ValidTransitions(t *testing.T) {
    fsm := NewPlanExecuteFSM("task-123", 5)
    
    if fsm.State() != distagentv1.TaskState_TASK_STATE_PENDING {
        t.Fatalf("expected PENDING, got %v", fsm.State())
    }
    
    err := fsm.Transition(distagentv1.TaskState_TASK_STATE_PLANNING)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    err = fsm.Transition(distagentv1.TaskState_TASK_STATE_PLAN_GENERATED)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    err = fsm.Transition(distagentv1.TaskState_TASK_STATE_STEP_EXECUTING)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    err = fsm.Transition(distagentv1.TaskState_TASK_STATE_STEP_COMPLETED)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    if fsm.StepCount() != 1 {
        t.Fatalf("expected step count 1, got %d", fsm.StepCount())
    }
    
    err = fsm.Transition(distagentv1.TaskState_TASK_STATE_FINAL_ANSWER)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    if !fsm.IsTerminal() {
        t.Fatal("expected FSM to be terminal")
    }
}

func TestPlanExecuteFSM_InvalidTransitions(t *testing.T) {
    fsm := NewPlanExecuteFSM("task-123", 5)
    
    err := fsm.Transition(distagentv1.TaskState_TASK_STATE_STEP_EXECUTING)
    if err == nil {
        t.Fatal("expected error for invalid transition, got nil")
    }
    
    if fsm.State() != distagentv1.TaskState_TASK_STATE_PENDING {
        t.Fatalf("expected FSM to remain PENDING, got %v", fsm.State())
    }
}
