package di_test

import (
	"testing"

	"github.com/saifsilver/goplusplus/di"
)

type DatabaseService struct{ DSN string }

func TestDIContainerLifecycle(t *testing.T) {
	container := di.New()

	dbService := &DatabaseService{DSN: "postgres://..."}
	container.Provide(dbService)

	started := false
	stopped := false

	container.OnStart(func() error {
		started = true
		return nil
	})

	container.OnStop(func() error {
		stopped = true
		return nil
	})

	if err := container.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !started {
		t.Errorf("expected OnStart hook to execute")
	}

	if err := container.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if !stopped {
		t.Errorf("expected OnStop hook to execute")
	}
}
