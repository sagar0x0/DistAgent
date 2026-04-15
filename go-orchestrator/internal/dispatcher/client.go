package dispatcher

import (
	"context"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

// AgentClient handles the connection to a single python agent worker
type AgentClient struct {
	addr   string
	conn   *grpc.ClientConn
	client distagentv1.AgentWorkerServiceClient
}

// NewAgentClient dials the python agent on the specified address
func NewAgentClient(addr string) (*AgentClient, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	
	client := distagentv1.NewAgentWorkerServiceClient(conn)
	
	return &AgentClient{
		addr:   addr,
		conn:   conn,
		client: client,
	}, nil
}

// ExecuteTask triggers the ReAct loop on the Python agent and streams responses back
func (c *AgentClient) ExecuteTask(ctx context.Context, req *distagentv1.ExecuteTaskRequest, outputChan chan<- *distagentv1.ExecuteTaskResponse) error {
	defer close(outputChan) // close channel when stream is done

	stream, err := c.client.ExecuteTask(ctx, req)
	if err != nil {
		return err
	}
	
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		
		outputChan <- resp
		
		if resp.State == distagentv1.TaskState_TASK_STATE_FINAL_ANSWER ||
		   resp.State == distagentv1.TaskState_TASK_STATE_FAILED ||
		   resp.State == distagentv1.TaskState_TASK_STATE_CANCELLED {
			break // Terminal state reached
		}
	}
	
	return nil
}

// Close gracefully closes the gRPC connection
func (c *AgentClient) Close() error {
	return c.conn.Close()
}
