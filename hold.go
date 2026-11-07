package main

import (
	"math"
	"sync"
	"time"
)

// A copy waiting out the hold before it's sent. Closing cancel calls it off.
type pendingCopy struct {
	id     int
	cancel chan struct{}
}

// Copies waiting out the hold, oldest first. Undo takes the newest; stopping takes them all.
type holdQueue struct {
	interval time.Duration // Between countdown ticks: a second in the app, far less in tests
	mu       sync.Mutex
	pending  []pendingCopy
}

var pendingSends = &holdQueue{interval: time.Second}

// Waits out the hold for one copy, calling tick with the ticks left (seconds, in the app) and how many copies
// are waiting, then send. If the copy is undone or everything is cancelled first, cancelled is called instead.
// Runs in the caller's goroutine, one per copy.
func (q *holdQueue) hold(id int, d time.Duration, tick func(left, waiting int), send, cancelled func()) {
	entry := q.add(id)
	remaining := d
	for left := int(math.Ceil(float64(d) / float64(q.interval))); left > 0; left-- {
		tick(left, q.waiting())
		step := min(remaining, q.interval)
		select {
		case <-time.After(step):
			remaining -= step
		case <-entry.cancel:
			cancelled()
			return
		}
	}
	// The hold can run out just as the copy is cancelled, so the queue has the last word
	if !q.remove(id) {
		cancelled()
		return
	}
	send()
}

func (q *holdQueue) add(id int) pendingCopy {
	q.mu.Lock()
	defer q.mu.Unlock()
	entry := pendingCopy{id: id, cancel: make(chan struct{})}
	q.pending = append(q.pending, entry)
	return entry
}

// Takes a copy out of the queue once its hold is over. False if it was cancelled in the meantime.
func (q *holdQueue) remove(id int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, entry := range q.pending {
		if entry.id == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			return true
		}
	}
	return false
}

func (q *holdQueue) waiting() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

// Calls off the newest copy still waiting. False if none was.
func (q *holdQueue) undo() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	last := len(q.pending) - 1
	if last < 0 {
		return false
	}
	close(q.pending[last].cancel)
	q.pending = q.pending[:last]
	return true
}

// Calls off every copy still waiting and returns how many there were
func (q *holdQueue) cancelAll() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, entry := range q.pending {
		close(entry.cancel)
	}
	n := len(q.pending)
	q.pending = nil
	return n
}
