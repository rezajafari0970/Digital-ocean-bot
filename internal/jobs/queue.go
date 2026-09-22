package jobs

import "sync"

type Queue struct {
	mu    sync.Mutex
	items []Operation
}

func (q *Queue) Push(op Operation) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, op)
}

func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
