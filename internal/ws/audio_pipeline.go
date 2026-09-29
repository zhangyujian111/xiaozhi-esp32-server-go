package ws

import (
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/vad"
)

type AudioPipeline struct {
	sm        *SessionManager
	vad       vad.VAD
	service   *vad.Service
	eventCh   chan AudioEvent
	interrupt InterruptTrigger
}

type InterruptTrigger interface {
	Trigger()
}

type AudioEvent struct {
	SessionID string
	Status    vad.Status
}

func NewAudioPipeline(sm *SessionManager, v vad.VAD, speechTh, silenceTh float32, silenceMs int) *AudioPipeline {
	p := &AudioPipeline{
		sm:      sm,
		vad:     v,
		service: vad.NewService(v, speechTh, silenceTh, silenceMs),
		eventCh: make(chan AudioEvent, 100),
	}
	return p
}

func (p *AudioPipeline) SetInterruptTrigger(trigger InterruptTrigger) {
	p.interrupt = trigger
}

func (p *AudioPipeline) Feed(sessionID string, pcm []int16) {
	session, ok := p.sm.Get(sessionID)
	if !ok {
		return
	}

	status := p.service.Feed(pcm)
	if status == vad.SpeechStart && p.interrupt != nil {
		p.interrupt.Trigger()
	}
	if status != vad.Silence {
		evt := AudioEvent{SessionID: session.ID(), Status: status}
		select {
		case p.eventCh <- evt:
		default:
		}
	}
}

func (p *AudioPipeline) FeedBytes(sessionID string, data []byte) {
	pcm := bytesToInt16(data)
	p.Feed(sessionID, pcm)
}

func bytesToInt16(data []byte) []int16 {
	n := len(data) / 2
	pcm := make([]int16, n)
	for i := 0; i < n; i++ {
		pcm[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}
	return pcm
}

func (p *AudioPipeline) Events() <-chan AudioEvent {
	return p.eventCh
}
