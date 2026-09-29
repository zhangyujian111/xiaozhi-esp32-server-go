package aisaas

import (
	"sync"
	"time"
)

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

type Breaker struct {
	mu            sync.Mutex
	state         State
	failureCount  int
	successCount  int
	failureThresh int
	openDuration  time.Duration
	openedAt      time.Time
}

func NewBreaker(failureThresh int, openDuration time.Duration) *Breaker {
	return &Breaker{
		state:         Closed,
		failureThresh: failureThresh,
		openDuration:  openDuration,
	}
}

func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case Closed:
		return true
	case Open:
		if time.Since(b.openedAt) > b.openDuration {
			b.state = HalfOpen
			return true
		}
		return false
	case HalfOpen:
		return true
	}
	return false
}

func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.successCount++
	if b.state == HalfOpen {
		b.state = Closed
		b.failureCount = 0
	}
}

func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failureCount++
	if b.failureCount >= b.failureThresh {
		b.state = Open
		b.openedAt = time.Now()
	}
}
