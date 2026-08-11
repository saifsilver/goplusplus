package saga_test

import (
	"context"
	"errors"
	"testing"

	"github.com/saifsilver/goplusplus/saga"
)

func TestSagaCoordinatorSuccessAndCompensate(t *testing.T) {
	ctx := context.Background()

	// Case 1: Success path
	coord1 := saga.NewCoordinator()
	step1Executed := false
	coord1.AddStep("create_order", func(ctx context.Context) error {
		step1Executed = true
		return nil
	}, nil)

	if err := coord1.Execute(ctx); err != nil {
		t.Fatalf("expected saga execution to succeed, got %v", err)
	}
	if !step1Executed {
		t.Errorf("expected step1 to execute")
	}

	// Case 2: Failure path triggers reverse compensation
	coord2 := saga.NewCoordinator()
	compensatedStep1 := false

	coord2.AddStep("reserve_credit", func(ctx context.Context) error {
		return nil
	}, func(ctx context.Context) error {
		compensatedStep1 = true
		return nil
	})

	coord2.AddStep("charge_card", func(ctx context.Context) error {
		return errors.New("card declined")
	}, nil)

	err2 := coord2.Execute(ctx)
	if err2 == nil {
		t.Fatalf("expected saga execution to fail")
	}
	if !compensatedStep1 {
		t.Errorf("expected step 1 to be compensated in reverse order")
	}
}
