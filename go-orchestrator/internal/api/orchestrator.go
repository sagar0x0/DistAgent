package api

import (
	"context"
	"fmt"
	"log"
	"sync"

	"connectrpc.com/connect"
	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
	"github.com/sagar/distagent/internal/stategraph"
	"github.com/sagar/distagent/internal/dispatcher"
	"github.com/sagar/distagent/internal/registry"
	"github.com/sagar/distagent/internal/router"
)

type OrchestratorServer struct {
	agentRegistry *registry.AgentRegistry
	router        *router.InferenceRouter
	jobCounter    int
	mu            sync.Mutex
}

func NewOrchestratorServer(ar *registry.AgentRegistry, r *router.InferenceRouter) *OrchestratorServer {
	return &OrchestratorServer{
		agentRegistry: ar,
		router:        r,
	}
}

func (s *OrchestratorServer) SubmitJob(
	ctx context.Context,
	req *connect.Request[distagentv1.SubmitJobRequest],
	stream *connect.ServerStream[distagentv1.SubmitJobResponse],
) error {
	s.mu.Lock()
	s.jobCounter++
	jobID := fmt.Sprintf("job-%d", s.jobCounter)
	s.mu.Unlock()

	// Initial Acknowledgment
	_ = stream.Send(&distagentv1.SubmitJobResponse{
		JobId:  jobID,
		NodeId: "root",
		Event: &distagentv1.SubmitJobResponse_Status{
			Status: &distagentv1.StatusEvent{
				State:   distagentv1.TaskState_TASK_STATE_PENDING,
				Message: "Job received. Building execution DAG...",
			},
		},
	})

	// 1. Build State Graph Components
	initialState := &distagentv1.WorkflowState{
		SessionId: req.Msg.SessionId,
		SharedContextJson: `{}`,
	}
	
	supervisor := stategraph.NewSupervisor()
	exec := stategraph.NewStateGraphExecutor(supervisor)
	
	targetModel := "gemini-3.1-flash-lite-preview" 

	// 2. Dispatch Function for StateGraph loops
	dispatchFn := func(execCtx context.Context, nodeID string, state *distagentv1.WorkflowState) (*distagentv1.WorkflowState, error) {
		systemPrompt := "You are an AI."
		var allowedTools []*distagentv1.ToolDefinition

		prefixHash := router.ComputePrefixHash(systemPrompt, allowedTools)

		// A. Inference Router
		infConfig, err := s.router.Route(targetModel, req.Msg.SessionId, prefixHash)
		if err != nil {
			return state, fmt.Errorf("router failed: %w", err)
		}

		// B. Least Loaded Agent 
		agentHb, err := s.agentRegistry.GetLeastLoaded()
		if err != nil {
			return state, fmt.Errorf("no agents available: %w", err)
		}

		// C. Network Dispatch
		log.Printf("Dispatching node '%s' to Agent '%s' (%s) [Prefix: %s]", nodeID, agentHb.AgentId, agentHb.AgentAddress, prefixHash)
		client, err := dispatcher.NewAgentClient(agentHb.AgentAddress)
		if err != nil {
			return state, fmt.Errorf("failed to dial agent: %w", err)
		}
		defer client.Close()

		agentReq := &distagentv1.ExecuteTaskRequest{
			TaskId:             fmt.Sprintf("%s-%s", jobID, nodeID),
			JobId:              jobID,
			SystemPrompt:       systemPrompt,
			UserContext:        req.Msg.UserPrompt,
			AllowedTools:       allowedTools,
			MaxReactSteps:      5,
			ExecutorInference:  infConfig,
			PlannerInference:   infConfig, // Usually a larger model for planning
			CurrentState:       state,
			CurrentNode:        &distagentv1.AgentNode{NodeId: nodeID, AgentType: "generic"},
		}

		outChan := make(chan *distagentv1.ExecuteTaskResponse)
		var dispatchErr error

		go func() {
			err := client.ExecuteTask(execCtx, agentReq, outChan)
			if err != nil {
				dispatchErr = err
			}
		}()

		// D. Map Agent telemetry directly to REST/gRPC client
		for resp := range outChan {
			var event *distagentv1.SubmitJobResponse
			
			if resp.GetThought() != nil {
				event = &distagentv1.SubmitJobResponse{
					JobId: jobID, NodeId: nodeID,
					Event: &distagentv1.SubmitJobResponse_Thinking{
						Thinking: &distagentv1.ThinkingEvent{
							ThoughtText: resp.GetThought().Text,
							StepNumber:  resp.GetThought().Step,
						},
					},
				}
			} else if resp.GetAction() != nil {
				event = &distagentv1.SubmitJobResponse{
					JobId: jobID, NodeId: nodeID,
					Event: &distagentv1.SubmitJobResponse_Action{
						Action: &distagentv1.ActionEvent{
							ToolName:      resp.GetAction().ToolName,
							ToolCallId:    resp.GetAction().ToolCallId,
							ArgumentsJson: resp.GetAction().ArgumentsJson,
							StepNumber:    resp.GetAction().Step,
						},
					},
				}
			} else if resp.GetFinalResult() != nil {
				event = &distagentv1.SubmitJobResponse{
					JobId: jobID, NodeId: nodeID,
					Event: &distagentv1.SubmitJobResponse_FinalAnswer{
						FinalAnswer: &distagentv1.FinalAnswerEvent{
							AnswerText: resp.GetFinalResult().Answer,
							TotalSteps: resp.GetFinalResult().TotalSteps,
						},
					},
				}
			} else if resp.GetError() != nil {
				return state, fmt.Errorf("node %s failed: %s", nodeID, resp.GetError().Message)
			}
			
			if resp.GetFinalResult() != nil && resp.GetFinalResult().StateUpdateJson != "" {
				state.SharedContextJson = resp.GetFinalResult().StateUpdateJson
			}
			
			if event != nil {
				_ = stream.Send(event)
			}
		}
		return state, dispatchErr
	}

	// 3. Trigger StateGraph Supervisor Routing execution
	err := exec.Execute(ctx, "root_task", initialState, dispatchFn)
	if err != nil {
		_ = stream.Send(&distagentv1.SubmitJobResponse{
			JobId: jobID,
			Event: &distagentv1.SubmitJobResponse_Status{
				Status: &distagentv1.StatusEvent{
					State: distagentv1.TaskState_TASK_STATE_FAILED, Message: err.Error(),
				},
			},
		})
		return err
	}
	return nil
}

func (s *OrchestratorServer) GetJobStatus(
	ctx context.Context,
	req *connect.Request[distagentv1.GetJobStatusRequest],
) (*connect.Response[distagentv1.GetJobStatusResponse], error) {
	return connect.NewResponse(&distagentv1.GetJobStatusResponse{
		JobId: req.Msg.JobId,
		State: distagentv1.TaskState_TASK_STATE_PENDING,
	}), nil
}

func (s *OrchestratorServer) CancelJob(
	ctx context.Context,
	req *connect.Request[distagentv1.CancelJobRequest],
) (*connect.Response[distagentv1.CancelJobResponse], error) {
	return connect.NewResponse(&distagentv1.CancelJobResponse{
		Acknowledged: true,
	}), nil
}
