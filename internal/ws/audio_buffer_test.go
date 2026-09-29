package ws

import (
	"testing"
)

func TestAudioRingBuffer_Write_Drain_Len(t *testing.T) {
	buf := NewAudioRingBuffer(16000, 5)

	if buf.Len() != 0 {
		t.Fatalf("initial Len() = %d, want 0", buf.Len())
	}

	testData := []byte{1, 2, 3, 4, 5}
	buf.Write(testData)
	if buf.Len() != len(testData) {
		t.Fatalf("after Write Len() = %d, want %d", buf.Len(), len(testData))
	}

	out := buf.Drain()
	if len(out) != len(testData) {
		t.Fatalf("Drain() returned %d bytes, want %d", len(out), len(testData))
	}
	for i := range testData {
		if out[i] != testData[i] {
			t.Errorf("Drain()[%d] = %d, want %d", i, out[i], testData[i])
		}
	}

	if buf.Len() != 0 {
		t.Fatalf("after Drain Len() = %d, want 0", buf.Len())
	}
}

func TestAudioRingBuffer_Overflow(t *testing.T) {
	buf := NewAudioRingBuffer(16000, 1)
	capacity := 16000 * 1 * 2

	fill := make([]byte, capacity+100)
	for i := range fill {
		fill[i] = byte(i % 256)
	}
	buf.Write(fill)

	if buf.Len() != capacity {
		t.Fatalf("after overflow Write Len() = %d, want capacity %d", buf.Len(), capacity)
	}
}

func TestAudioRingBuffer_MultipleWrite(t *testing.T) {
	buf := NewAudioRingBuffer(16000, 5)

	buf.Write([]byte{1, 2})
	buf.Write([]byte{3, 4})

	if buf.Len() != 4 {
		t.Fatalf("Len() = %d, want 4", buf.Len())
	}

	out := buf.Drain()
	if len(out) != 4 {
		t.Fatalf("Drain() len = %d, want 4", len(out))
	}
}
