package ws

import (
	"sync"

	"github.com/rs/zerolog"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/vad"
)

type AudioPipeline struct {
	sm        *SessionManager
	vad       vad.VAD
	service   *vad.Service
	eventCh   chan AudioEvent
	interrupt InterruptTrigger
	log       zerolog.Logger

	// Per-session VAD state (rolling 64-sample context + 128-dim LSTM state).
	// Lazily created on first Feed. Cleared via ResetSession.
	vadMu     sync.Mutex
	vSessions map[string]vad.Session
	speaking  map[string]bool // VAD SpeechStart ⇢ SpeechEnd flip per session

	// Diagnostic: how many probabilities we've already logged for a session.
	probCount map[string]int

	// Long-silence GRU reset (mirrors xiaozhi-java's SILENCE_RESET_FRAMES).
	silenceCounter      map[string]int
	silenceResetFrames  int
}

type InterruptTrigger interface {
	Trigger()
}

type AudioEvent struct {
	SessionID string
	Status    vad.Status
}

func NewAudioPipeline(sm *SessionManager, v vad.VAD, speechTh, silenceTh float32, silenceMs int) *AudioPipeline {
	return NewAudioPipelineWithLog(sm, v, speechTh, silenceTh, silenceMs, zerolog.Nop())
}

func NewAudioPipelineWithLog(sm *SessionManager, v vad.VAD, speechTh, silenceTh float32, silenceMs int, log zerolog.Logger) *AudioPipeline {
	return &AudioPipeline{
		sm:                 sm,
		vad:                v,
		service:            vad.NewService(v, speechTh, silenceTh, silenceMs),
		eventCh:            make(chan AudioEvent, 100),
		log:                log,
		vSessions:          make(map[string]vad.Session),
		speaking:           make(map[string]bool),
		probCount:          make(map[string]int),
		silenceCounter:     make(map[string]int),
		silenceResetFrames: 30,
	}
}

func (p *AudioPipeline) SetInterruptTrigger(trigger InterruptTrigger) {
	p.interrupt = trigger
}

// ResetSession drops the per-session VAD state. Call when a dialogue turn ends
// so a new turn starts with fresh LSTM context.
func (p *AudioPipeline) ResetSession(sessionID string) {
	p.vadMu.Lock()
	defer p.vadMu.Unlock()
	delete(p.vSessions, sessionID)
	delete(p.speaking, sessionID)
	delete(p.probCount, sessionID)
	delete(p.silenceCounter, sessionID)
	p.service.Reset()
}

// IsSpeaking reports whether the VAD state machine is currently in the
// SpeechContinue state for the given session. Used by the handler to decide
// when to start/stop accumulating PCM into the audio buffer for STT.
func (p *AudioPipeline) IsSpeaking(sessionID string) bool {
	p.vadMu.Lock()
	defer p.vadMu.Unlock()
	return p.speaking[sessionID]
}

func (p *AudioPipeline) sessionFor(id string) vad.Session {
	p.vadMu.Lock()
	defer p.vadMu.Unlock()
	if s, ok := p.vSessions[id]; ok {
		return s
	}
	s := p.vad.StartSession()
	p.vSessions[id] = s
	return s
}

func (p *AudioPipeline) Feed(sessionID string, pcm []int16) {
	session, ok := p.sm.Get(sessionID)
	if !ok {
		return
	}

	vs := p.sessionFor(sessionID)
	status := p.service.Feed(vs, pcm)

	// Track per-session speech state for handler-side decisions (when to start
	// accumulating PCM into the audio buffer for STT).
	switch status {
	case vad.SpeechStart:
		p.vadMu.Lock()
		p.speaking[sessionID] = true
		p.vadMu.Unlock()
		if p.interrupt != nil {
			p.interrupt.Trigger()
		}
	case vad.SpeechEnd:
		p.vadMu.Lock()
		delete(p.speaking, sessionID)
		p.vadMu.Unlock()
	}

	// Diagnostic: log first 30 probabilities per session so we can see whether
	// the model is firing (just above threshold) or staying silent (model
	// issue, mic gain, sample-rate, etc.).
	if p.probCount[sessionID] < 30 {
		p.probCount[sessionID]++
		p.log.Info().Str("session", sessionID).Float32("prob", p.service.LastProb()).Str("status", status.String()).Msg("vad probe")
	}

	// Long-silence GRU reset: after ~1 s of silence (30 frames @ 32 ms) the
	// LSTM hidden state can drift and stop responding; mirror xiaozhi-java's
	// SILENCE_RESET_FRAMES behaviour by dropping the per-session state.
	if status == vad.Silence && !p.service.IsSpeaking() {
		p.silenceCounter[sessionID]++
		if p.silenceCounter[sessionID] >= p.silenceResetFrames {
			p.vadMu.Lock()
			if s, ok := p.vSessions[sessionID]; ok {
				s.Reset()
			}
			p.vadMu.Unlock()
			p.silenceCounter[sessionID] = 0
		}
	} else {
		p.silenceCounter[sessionID] = 0
	}

	if status != vad.Silence {
		evt := AudioEvent{SessionID: session.ID(), Status: status}
		select {
		case p.eventCh <- evt:
		default:
			p.log.Warn().Str("session", sessionID).Msg("vad event channel full; event dropped")
		}
	}
}

func (p *AudioPipeline) logProbe() {}

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