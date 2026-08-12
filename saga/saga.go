package saga

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// Step defines one action and its optional reverse compensation.
type Step struct {
	Name       string
	Execute    func(ctx context.Context) error
	Compensate func(ctx context.Context) error
}

// Coordinator runs process-local saga steps with reverse compensation.
// It is not a durable workflow engine and cannot recover work after a process exit.
type Coordinator struct {
	steps []Step
}

// NewCoordinator creates a new saga coordinator instance.
func NewCoordinator() *Coordinator {
	return &Coordinator{}
}

// AddStep adds a transaction step with an execution func and reverse compensation func.
func (c *Coordinator) AddStep(name string, execute, compensate func(ctx context.Context) error) {
	c.steps = append(c.steps, Step{
		Name:       name,
		Execute:    execute,
		Compensate: compensate,
	})
}

// Execute runs steps in order and compensates completed steps in reverse order.
// The returned error includes both the failed step and any compensation failures.
func (c *Coordinator) Execute(ctx context.Context) error {
	var executed []Step

	for _, step := range c.steps {
		if step.Execute == nil {
			return fmt.Errorf("saga: step %q has no execute function", step.Name)
		}
		slog.Info("saga: Executing transaction step", slog.String("step", step.Name))
		if err := step.Execute(ctx); err != nil {
			slog.Error("saga: Step failed; initiating automatic reverse compensation",
				slog.String("failed_step", step.Name),
				slog.String("error", err.Error()),
			)
			stepErr := fmt.Errorf("saga: step %q failed: %w", step.Name, err)
			if compensationErr := c.compensateReverse(ctx, executed); compensationErr != nil {
				return errors.Join(stepErr, compensationErr)
			}
			return stepErr
		}
		executed = append(executed, step)
	}

	slog.Info("saga: Distributed saga transaction completed successfully")
	return nil
}

func (c *Coordinator) compensateReverse(ctx context.Context, executed []Step) error {
	var compensationErrors []error
	for i := len(executed) - 1; i >= 0; i-- {
		step := executed[i]
		if step.Compensate != nil {
			slog.Warn("saga: Compensating step", slog.String("step", step.Name))
			if err := step.Compensate(ctx); err != nil {
				compensationErrors = append(compensationErrors, fmt.Errorf("saga: compensate step %q: %w", step.Name, err))
			}
		}
	}
	return errors.Join(compensationErrors...)
}
