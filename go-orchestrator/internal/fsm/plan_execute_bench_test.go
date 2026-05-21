package fsm

import (
	"fmt"
	"testing"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

// BenchmarkFSM_FullLifecycle measures throughput of entire plan-and-execute lifecycle
func BenchmarkFSM_FullLifecycle(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		fsm := NewPlanExecuteFSM(fmt.Sprintf("task-%d", i), 5)
		
		_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_PLANNING)
		_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_PLAN_GENERATED)
		
		// Micro step 1
		_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_STEP_EXECUTING)
		_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_STEP_COMPLETED)
		
		// Micro step 2
		_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_STEP_EXECUTING)
		_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_STEP_COMPLETED)
		
		_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_FINAL_ANSWER)
	}
}

// BenchmarkFSM_SingleTransition isolates the cost of one state validation/transition
func BenchmarkFSM_SingleTransition(b *testing.B) {
	fsm := NewPlanExecuteFSM("task-single", 100000000) // Huge limit to avoid failing
	
	// Pre-condition state
	_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_PLANNING)
	_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_PLAN_GENERATED)
	
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Ping pong between executing and completed
		_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_STEP_EXECUTING)
		_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_STEP_COMPLETED)
	}
}

// BenchmarkFSM_Parallel measures FSM lifecycle throughput under contention
func BenchmarkFSM_Parallel(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			fsm := NewPlanExecuteFSM(fmt.Sprintf("task-p-%d", i), 5)
			
			_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_PLANNING)
			_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_PLAN_GENERATED)
			
			// Micro step 1
			_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_STEP_EXECUTING)
			_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_STEP_COMPLETED)
			
			_ = fsm.Transition(distagentv1.TaskState_TASK_STATE_FINAL_ANSWER)
			i++
		}
	})
}
