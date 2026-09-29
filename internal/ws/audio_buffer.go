package ws

import "sync"

type AudioRingBuffer struct {
	mu   sync.Mutex
	data []byte
	cap  int
}

func NewAudioRingBuffer(sampleRate int, maxSeconds int) *AudioRingBuffer {
	return &AudioRingBuffer{
		data: make([]byte, 0, sampleRate*maxSeconds*2),
		cap:  sampleRate * maxSeconds * 2,
	}
}

func (b *AudioRingBuffer) Write(p []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > b.cap {
		b.data = b.data[len(b.data)-b.cap:]
	}
}

func (b *AudioRingBuffer) Drain() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, len(b.data))
	copy(out, b.data)
	b.data = b.data[:0]
	return out
}

func (b *AudioRingBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.data)
}

func (b *AudioRingBuffer) WritePCM(pcm []int16) {
	buf := make([]byte, len(pcm)*2)
	for i, s := range pcm {
		buf[i*2] = byte(s)
		buf[i*2+1] = byte(s >> 8)
	}
	b.Write(buf)
}
