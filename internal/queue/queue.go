package queue

import (
	"sync"
	"sync/atomic"

	"github.com/ms/agent-daemon/internal/types"
)

// BoundedQueue manages concurrent job execution with dedup.
type BoundedQueue struct {
	pending   chan *types.Job
	semaphore chan struct{}
	running   sync.Map // jobID → struct{}
	active    atomic.Int32
	execute   func(job *types.Job)
	done      chan struct{}
	wg        sync.WaitGroup
}

func New(maxParallel, queueSize int, execute func(job *types.Job)) *BoundedQueue {
	q := &BoundedQueue{
		pending:   make(chan *types.Job, queueSize),
		semaphore: make(chan struct{}, maxParallel),
		execute:   execute,
		done:      make(chan struct{}),
	}
	q.wg.Add(1)
	go q.dispatcher()
	return q
}

// Submit enqueues a job. Returns false if the job is already running (dedup) or queue is full.
func (q *BoundedQueue) Submit(job *types.Job) bool {
	if _, loaded := q.running.LoadOrStore(job.ID, struct{}{}); loaded {
		// already running — dedup
		q.running.Delete(job.ID) // undo the store since it was loaded
		return false
	}
	q.running.Delete(job.ID) // remove the probe store; will be set again by dispatcher
	select {
	case q.pending <- job:
		return true
	default:
		// queue full
		return false
	}
}

// IsRunning returns whether the job is currently executing.
func (q *BoundedQueue) IsRunning(jobID string) bool {
	_, ok := q.running.Load(jobID)
	return ok
}

// ActiveCount returns the number of currently executing jobs.
func (q *BoundedQueue) ActiveCount() int { return int(q.active.Load()) }

// PendingCount returns the number of jobs waiting in the queue.
func (q *BoundedQueue) PendingCount() int { return len(q.pending) }

// Flush drains all pending (not yet executing) jobs from the queue.
func (q *BoundedQueue) Flush() {
	for {
		select {
		case <-q.pending:
		default:
			return
		}
	}
}

// Stop shuts down the dispatcher. Waits for in-flight jobs to complete.
func (q *BoundedQueue) Stop() {
	close(q.done)
	q.wg.Wait()
}

func (q *BoundedQueue) dispatcher() {
	defer q.wg.Done()
	for {
		select {
		case <-q.done:
			return
		case job := <-q.pending:
			// Dedup check: if same job somehow submitted twice before first starts
			if _, loaded := q.running.LoadOrStore(job.ID, struct{}{}); loaded {
				continue // skip duplicate
			}
			q.semaphore <- struct{}{} // acquire slot (blocks if at max parallel)
			q.active.Add(1)
			q.wg.Add(1)
			go func(j *types.Job) {
				defer func() {
					q.running.Delete(j.ID)
					<-q.semaphore
					q.active.Add(-1)
					q.wg.Done()
				}()
				q.execute(j)
			}(job)
		}
	}
}
