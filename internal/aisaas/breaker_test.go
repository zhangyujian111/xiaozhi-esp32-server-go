package aisaas

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBreaker_InitialClosed_Allows(t *testing.T) {
	b := NewBreaker(3, 10*time.Second)
	assert.NotNil(t, b)
	assert.True(t, b.Allow())
}

func TestBreaker_OpenAfterThresholdFailures(t *testing.T) {
	b := NewBreaker(3, 10*time.Second)

	for i := 0; i < 2; i++ {
		b.RecordFailure()
	}
	require.True(t, b.Allow(), "should still allow after 2 failures (threshold=3)")

	b.RecordFailure()
	assert.False(t, b.Allow(), "should be open after 3 failures")
}

func TestBreaker_OpenRejects(t *testing.T) {
	b := NewBreaker(2, 10*time.Second)

	b.RecordFailure()
	b.RecordFailure()
	require.False(t, b.Allow(), "should be open")

	for i := 0; i < 5; i++ {
		assert.False(t, b.Allow(), "should reject when open")
	}
}

func TestBreaker_HalfOpenAfterDuration(t *testing.T) {
	b := NewBreaker(2, 50*time.Millisecond)

	b.RecordFailure()
	b.RecordFailure()
	require.False(t, b.Allow())

	time.Sleep(60 * time.Millisecond)
	assert.True(t, b.Allow(), "should transition to half-open after duration")
}

func TestBreaker_HalfOpenSuccessCloses(t *testing.T) {
	b := NewBreaker(2, 50*time.Millisecond)

	b.RecordFailure()
	b.RecordFailure()
	require.False(t, b.Allow())

	time.Sleep(60 * time.Millisecond)
	require.True(t, b.Allow(), "should be half-open")

	b.RecordSuccess()
	assert.True(t, b.Allow(), "should be closed after half-open success")

	b2 := NewBreaker(2, 50*time.Millisecond)
	b2.state = HalfOpen
	b2.RecordSuccess()
	assert.Equal(t, Closed, b2.state)
}

func TestBreaker_HalfOpenFailureReopens(t *testing.T) {
	b := NewBreaker(2, 50*time.Millisecond)

	b.RecordFailure()
	b.RecordFailure()
	require.False(t, b.Allow())

	time.Sleep(60 * time.Millisecond)
	require.True(t, b.Allow())

	b.RecordFailure()
	assert.False(t, b.Allow(), "should reopen after half-open failure")
}

func TestBreaker_Concurrent(t *testing.T) {
	b := NewBreaker(100, 10*time.Second)
	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Allow()
			b.RecordSuccess()
			b.RecordFailure()
		}()
	}
	wg.Wait()
}
