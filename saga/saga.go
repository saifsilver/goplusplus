package saga

import (
	"context"
	"fmt"
	"log/slog"
)

type Step struct {
	Name       string
	Execute    func(ctx context.Context) error
	Compensate func(ctx context.Context) error
}

// Coordinator manages multi-microservice distributed transactions with automatic reverse compensation on failure (Uber Cadence style).
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

// Execute executes all saga steps in order. If any step fails, it executes compensation functions in reverse order!
func (c *Coordinator) Execute(ctx context.Context) error {
	var executed []Step

	for _, step := range c.steps {
		slog.Info("saga: Executing transaction step", slog.String("step", step.Name))
		if err := step.Execute(ctx); err != nil {
			slog.Error("saga: Step failed; initiating automatic reverse compensation",
				slog.String("failed_step", step.Name),
				slog.String("error", err.Error()),
			)
			c.compensateReverse(ctx, executed)
			return fmt.Errorf("saga: step '%s' failed (reverse compensation completed): %w", step.Name, err)
		}
		executed = append(executed, step)
	}

	slog.Info("saga: Distributed saga transaction completed successfully")
	return nil
}

func (c *Coordinator) compensateReverse(ctx context.Context, executed []Step) {
	for i := len(executed) - 1; i >= 0; i-- {
		step := executed[i]
		if step.Compensate != nil {
			slog.Warn("saga: Compensating step", slog.String("step", step.Name))
			_ = step.Compensate(ctx)
		}
	}
}
