package saga

import (
	"context"
	"time"
)

// SagaState represents the lifecycle state of a saga transaction.
type SagaState string

const (
	SagaPending      SagaState = "PENDING"
	SagaRunning      SagaState = "RUNNING"
	SagaCompleted    SagaState = "COMPLETED"
	SagaCompensating SagaState = "COMPENSATING"
	SagaFailed       SagaState = "FAILED"
)

// Step is a saga unit of work with a compensating action.
type Step interface {
	Execute(ctx context.Context) error
	Compensate(ctx context.Context) error
}

// SagaLog is the persisted record of a saga execution.
type SagaLog struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	State       SagaState `json:"state"`
	Steps       []string  `json:"steps"`
	CurrentStep int       `json:"currentStep"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// SagaStore persists and loads saga logs for recovery/observability.
type SagaStore interface {
	Save(ctx context.Context, log *SagaLog) error
	Get(ctx context.Context, id string) (*SagaLog, error)
	Update(ctx context.Context, log *SagaLog) error
}

