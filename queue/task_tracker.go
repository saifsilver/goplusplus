package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// TaskStatus identifies process-local task execution state.
type TaskStatus string

const (
	// StatusPending indicates a registered task not yet running.
	StatusPending TaskStatus = "PENDING"
	// StatusRunning indicates an active task attempt.
	StatusRunning TaskStatus = "RUNNING"
	// StatusCompleted indicates successful process-local completion.
	StatusCompleted TaskStatus = "COMPLETED"
	// StatusFailed indicates exhausted retries.
	StatusFailed TaskStatus = "FAILED"
	// StatusRetrying indicates a subsequent attempt is running.
	StatusRetrying TaskStatus = "RETRYING"
)

// TaskInfo stores complete execution status, retries, and error details for a background task.
type TaskInfo struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Status      TaskStatus `json:"status"`
	Retries     int        `json:"retries"`
	MaxRetries  int        `json:"max_retries"`
	LastError   string     `json:"last_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// TaskTracker manages process-local background task statuses and retries.
// State is lost on restart; use a durable queue for production work.
type TaskTracker struct {
	mu    sync.RWMutex
	tasks map[string]*TaskInfo
}

var globalTracker = NewTaskTracker()

// NewTaskTracker creates a new task tracker instance.
func NewTaskTracker() *TaskTracker {
	return &TaskTracker{
		tasks: make(map[string]*TaskInfo),
	}
}

// AsyncTask dispatches a process-local background task with automatic retries.
func AsyncTask(name string, maxRetries int, fn func(ctx context.Context) error) string {
	return globalTracker.Dispatch(name, maxRetries, fn)
}

// GetTaskStatus retrieves the status and execution details of a background task by ID.
func GetTaskStatus(taskID string) (*TaskInfo, bool) {
	return globalTracker.GetStatus(taskID)
}

// Dispatch registers and executes a background task in a managed goroutine.
func (tt *TaskTracker) Dispatch(name string, maxRetries int, fn func(ctx context.Context) error) string {
	if maxRetries <= 0 {
		maxRetries = 3
	}

	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	info := &TaskInfo{
		ID:         taskID,
		Name:       name,
		Status:     StatusPending,
		MaxRetries: maxRetries,
		CreatedAt:  time.Now(),
	}

	tt.mu.Lock()
	tt.tasks[taskID] = info
	tt.mu.Unlock()

	go func(id string) {
		ctx := context.Background()

		for attempt := 0; attempt <= maxRetries; attempt++ {
			tt.mu.Lock()
			if attempt > 0 {
				tt.tasks[id].Status = StatusRetrying
				tt.tasks[id].Retries = attempt
			} else {
				tt.tasks[id].Status = StatusRunning
			}
			tt.mu.Unlock()

			slog.Info("queue: Executing tracked background task", slog.String("task_id", id), slog.Int("attempt", attempt+1))

			err := tt.executeTask(ctx, fn)
			if err == nil {
				tt.mu.Lock()
				tt.tasks[id].Status = StatusCompleted
				now := time.Now()
				tt.tasks[id].CompletedAt = &now
				tt.mu.Unlock()
				slog.Info("queue: Background task completed successfully", slog.String("task_id", id))
				return
			}

			tt.mu.Lock()
			tt.tasks[id].LastError = err.Error()
			tt.mu.Unlock()

			slog.Warn("queue: Background task attempt failed", slog.String("task_id", id), slog.String("error", err.Error()))

			if attempt < maxRetries {
				time.Sleep(time.Duration(100*(attempt+1)) * time.Millisecond) // Backoff
			}
		}

		tt.mu.Lock()
		tt.tasks[id].Status = StatusFailed
		tt.mu.Unlock()
		slog.Error("queue: Background task failed permanently after max retries", slog.String("task_id", id))
	}(taskID)

	return taskID
}

func (tt *TaskTracker) executeTask(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("task panic: %v", r)
		}
	}()
	return fn(ctx)
}

// GetStatus retrieves current task info by ID.
func (tt *TaskTracker) GetStatus(taskID string) (*TaskInfo, bool) {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	info, ok := tt.tasks[taskID]
	if !ok {
		return nil, false
	}
	cp := *info
	return &cp, true
}
