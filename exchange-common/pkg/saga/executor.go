package saga

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type Executor struct {
	store SagaStore
}

func NewExecutor(store SagaStore) *Executor {
	return &Executor{store: store}
}

// Run 执行 saga，失败时自动补偿
func (e *Executor) Run(ctx context.Context, name string, steps []Step) error {
	now := time.Now()
	log := &SagaLog{
		ID:        newID(),
		Name:      name,
		State:     SagaRunning,
		Steps:     make([]string, len(steps)),
		CreatedAt: now,
		UpdatedAt: now,
	}
	for i := range steps {
		log.Steps[i] = fmt.Sprintf("step-%d", i)
	}
	if err := e.store.Save(ctx, log); err != nil {
		return err
	}

	for i, step := range steps {
		log.CurrentStep = i
		log.UpdatedAt = time.Now()
		if err := e.store.Update(ctx, log); err != nil {
			log.Error = err.Error()
			log.State = SagaCompensating
			log.UpdatedAt = time.Now()
			_ = e.store.Update(ctx, log)

			var compErr error
			for j := i - 1; j >= 0; j-- {
				if e := steps[j].Compensate(ctx); e != nil && compErr == nil {
					compErr = e
				}
			}

			log.State = SagaFailed
			log.UpdatedAt = time.Now()
			_ = e.store.Update(ctx, log)

			if compErr != nil {
				return fmt.Errorf("saga log update failed: %w; compensate failed: %v", err, compErr)
			}
			return fmt.Errorf("saga log update failed: %w", err)
		}
		if err := step.Execute(ctx); err != nil {
			log.Error = err.Error()
			log.State = SagaCompensating
			log.UpdatedAt = time.Now()
			_ = e.store.Update(ctx, log)

			var compErr error
			for j := i - 1; j >= 0; j-- {
				if e := steps[j].Compensate(ctx); e != nil && compErr == nil {
					compErr = e
				}
			}

			log.State = SagaFailed
			log.UpdatedAt = time.Now()
			_ = e.store.Update(ctx, log)

			if compErr != nil {
				return fmt.Errorf("execute failed: %w; compensate failed: %v", err, compErr)
			}
			return err
		}
	}

	log.State = SagaCompleted
	log.CurrentStep = len(steps)
	log.Error = ""
	log.UpdatedAt = time.Now()
	return e.store.Update(ctx, log)
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
