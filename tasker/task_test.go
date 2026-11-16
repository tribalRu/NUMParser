package tasker

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestTaskerExecution(t *testing.T) {
	tsk := New(4, false)
	tsk.DisableLog()

	var counter atomic.Int32
	for i := 0; i < 20; i++ {
		tsk.Add(func(data interface{}) bool {
			counter.Add(1)
			return true
		}, nil)
	}

	go tsk.Run()
	tsk.Wait()

	if counter.Load() != 20 {
		t.Fatalf("expected counter 20, got %d", counter.Load())
	}
}

func TestTaskerEarlyStop(t *testing.T) {
	tsk := New(1, false)
	tsk.DisableLog()

	var executed atomic.Int32
	for i := 0; i < 20; i++ {
		idx := i
		tsk.Add(func(data interface{}) bool {
			executed.Add(1)
			if idx == 2 {
				return false // stop on 3rd task
			}
			return true
		}, nil)
	}

	doneCh := make(chan struct{})
	go func() {
		tsk.Run()
		tsk.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		// Should complete quickly and not hang
		if executed.Load() > 5 {
			t.Errorf("expected early stop, but executed %d tasks", executed.Load())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("tasker hung and failed to unblock on early stop")
	}
}
