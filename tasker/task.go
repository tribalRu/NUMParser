package tasker

import (
	"NUMParser/utils"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
)

type Tasker struct {
	tasks      []worker
	threads    int
	shuffle    bool
	disableLog bool
	isStop     atomic.Bool
	done       chan struct{}
	closeOnce  sync.Once
	mu         sync.Mutex
}

type worker struct {
	Func func(interface{}) bool
	Data interface{}
}

func New(threads int, shuffle bool) *Tasker {
	t := &Tasker{
		threads: threads,
		shuffle: shuffle,
		done:    make(chan struct{}),
	}
	if t.threads < 1 {
		t.threads = 1
	}
	return t
}

func (t *Tasker) DisableLog() {
	t.disableLog = true
}

func (t *Tasker) Stop() {
	t.isStop.Store(true)
}

func (t *Tasker) Add(fn func(interface{}) bool, data interface{}) {
	t.mu.Lock()
	t.tasks = append(t.tasks, worker{
		Func: fn,
		Data: data,
	})
	t.mu.Unlock()
}

func (t *Tasker) Run() {
	defer t.closeOnce.Do(func() {
		close(t.done)
	})

	if len(t.tasks) == 0 {
		return
	}

	if t.shuffle {
		rand.Shuffle(len(t.tasks), func(i, j int) { t.tasks[i], t.tasks[j] = t.tasks[j], t.tasks[i] })
	}

	utils.PForLim(t.tasks, t.threads, func(i int, wrk worker) bool {
		if t.isStop.Load() {
			return false
		}
		if !t.disableLog {
			log.Println("Task", i+1, "/", len(t.tasks))
		}
		if !wrk.Func(wrk.Data) {
			t.isStop.Store(true)
			return false
		}
		return !t.isStop.Load()
	})
}

func (t *Tasker) Wait() {
	<-t.done
}
