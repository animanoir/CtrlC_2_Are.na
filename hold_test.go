package main

import (
	"slices"
	"sync"
	"testing"
	"time"
)

// Runs copies through a queue with fast ticks and records what became of each, with fake senders instead of Are.na
type holdRun struct {
	q         *holdQueue
	wg        sync.WaitGroup
	started   chan int // Each copy's id, once its countdown has begun
	mu        sync.Mutex
	ticks     map[int][]int // Ticks left, as reported, per copy
	waiting   map[int]int   // The most copies reported waiting during each copy's countdown
	sent      map[int]bool
	cancelled map[int]bool
}

func newHoldRun() *holdRun {
	return &holdRun{
		q:         &holdQueue{interval: 10 * time.Millisecond},
		started:   make(chan int, 8),
		ticks:     map[int][]int{},
		waiting:   map[int]int{},
		sent:      map[int]bool{},
		cancelled: map[int]bool{},
	}
}

// Starts a copy's hold in the background
func (r *holdRun) start(id int, d time.Duration) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		first := true
		r.q.hold(id, d,
			func(left, waiting int) {
				r.mu.Lock()
				r.ticks[id] = append(r.ticks[id], left)
				r.waiting[id] = max(r.waiting[id], waiting)
				r.mu.Unlock()
				if first {
					first = false
					r.started <- id
				}
			},
			func() { r.mu.Lock(); r.sent[id] = true; r.mu.Unlock() },
			func() { r.mu.Lock(); r.cancelled[id] = true; r.mu.Unlock() },
		)
	}()
}

func (r *holdRun) outcome(id int) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case r.sent[id] && r.cancelled[id]:
		return "both sent and cancelled"
	case r.sent[id]:
		return "sent"
	case r.cancelled[id]:
		return "cancelled"
	}
	return "nothing"
}

func TestHoldSendsWhenTheHoldRunsOut(t *testing.T) {
	r := newHoldRun()
	r.start(1, 35*time.Millisecond) // Three and a half ticks, so the countdown starts at four
	<-r.started
	r.wg.Wait()

	if got := r.outcome(1); got != "sent" {
		t.Errorf("copy 1 was %s, want sent", got)
	}
	if got := r.ticks[1]; !slices.Equal(got, []int{4, 3, 2, 1}) {
		t.Errorf("ticks = %v, want 4 3 2 1", got)
	}
	if n := r.q.waiting(); n != 0 {
		t.Errorf("%d copies still waiting after the send", n)
	}
}

func TestUndoCallsOffTheNewestCopy(t *testing.T) {
	r := newHoldRun()
	r.start(1, 200*time.Millisecond)
	<-r.started
	r.start(2, 200*time.Millisecond)
	<-r.started

	if !r.q.undo() {
		t.Fatal("undo found nothing waiting")
	}
	r.wg.Wait()

	if got := r.outcome(2); got != "cancelled" {
		t.Errorf("the newest copy was %s, want cancelled", got)
	}
	if got := r.outcome(1); got != "sent" {
		t.Errorf("the older copy was %s, want sent", got)
	}
	if r.waiting[2] != 2 { // Copy 2 joined while copy 1 was still waiting, so its first tick saw both
		t.Errorf("copy 2 saw at most %d waiting, want 2", r.waiting[2])
	}
	if r.q.undo() {
		t.Error("undo with nothing waiting reported a copy")
	}
}

func TestStopCallsOffEveryCopy(t *testing.T) {
	r := newHoldRun()
	r.start(1, time.Second)
	<-r.started
	r.start(2, time.Second)
	<-r.started

	if n := r.q.cancelAll(); n != 2 {
		t.Errorf("cancelAll reported %d copies, want 2", n)
	}
	r.wg.Wait()

	for id := 1; id <= 2; id++ {
		if got := r.outcome(id); got != "cancelled" {
			t.Errorf("copy %d was %s, want cancelled", id, got)
		}
	}
	if n := r.q.waiting(); n != 0 {
		t.Errorf("%d copies still waiting after cancelAll", n)
	}
	if n := r.q.cancelAll(); n != 0 {
		t.Errorf("a second cancelAll reported %d copies, want 0", n)
	}
}
