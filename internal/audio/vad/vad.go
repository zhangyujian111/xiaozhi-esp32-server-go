package vad

type Status int

const (
	Silence Status = iota
	SpeechStart
	SpeechContinue
	SpeechEnd
	Error
)

func (s Status) String() string {
	switch s {
	case Silence:
		return "silence"
	case SpeechStart:
		return "speech_start"
	case SpeechContinue:
		return "speech_continue"
	case SpeechEnd:
		return "speech_end"
	case Error:
		return "error"
	default:
		return "unknown"
	}
}

// VAD is the shared, stateless handle to a VAD model (a single ONNX runtime
// session shared across all streams). Per-stream state is held in a Session.
type VAD interface {
	// Process runs inference on a fixed-size PCM chunk (typically 512 samples,
	// 16 kHz, mono). For stateful VAD models (VAD v5 with LSTM), callers must
	// use StartSession() and call Session.Process instead.
	Process(pcm []int16) (probability float32, err error)

	// StartSession opens a fresh per-stream state. Each Session owns its own
	// rolling context and LSTM state. Implementations that are inherently
	// stateless may return nil here, in which case callers fall back to
	// Process() (results may be inaccurate).
	StartSession() Session
}

// Session is the per-stream stateful handle to a VAD. Calls are serialised
// per Session; multiple sessions may safely run on the same VAD in parallel.
type Session interface {
	Process(pcm []int16) (probability float32, err error)
	Reset()
}

// SessionFactory is implemented by VAD implementations that lazily mint per-
// session state (used by Service to defer session allocation until first feed).
type SessionFactory interface {
	MintSession() Session
}

// Service is the state machine that turns raw VAD probabilities into
// SpeechStart / SpeechContinue / SpeechEnd / Silence events. The Service is
// itself stateless; the per-stream state is the Session supplied to Feed.
type Service struct {
	vad                       VAD
	speechTh                  float32
	silenceTh                 float32
	silenceFrames             int  // Frames of consecutive silence to fire SpeechEnd
	speechStartFrames         int  // Required consecutive speech frames to fire SpeechStart (default 2, matches xiaozhi-java)
	energyTh                  float32 // Optional energy threshold (matches Java method)
	inSpeech                  bool
	silenceCount              int
	speechCount               int
	lastProb                  float32
	lastErr                   error
}

// ServiceOptions holds all tunables for the VAD state machine. Zero values
// fall back to the Java defaults (speech=0.4, silence=0.3, energy=0.001,
// speechStartFrames=2, resetAfter=30, msPerFrame=32).
type ServiceOptions struct {
	SpeechThreshold     float32
	SilenceThreshold    float32
	EnergyThreshold     float32
	SilenceDurationMs   int
	SpeechStartFrames   int
	ResetAfterFrames    int // consecutive silence frames before resetting VAD state
	MsPerFrame          int // frames-per-ms used for silenceDurationMs -> frames. xiaozhi-java uses 10, VAD chunks are 32 ms (512 samples @ 16 kHz).
}

func NewService(v VAD, speechTh, silenceTh float32, silenceMs int) *Service {
	return NewServiceWithOptions(v, ServiceOptions{
		SpeechThreshold:   speechTh,
		SilenceThreshold:  silenceTh,
		SilenceDurationMs: silenceMs,
		SpeechStartFrames: 2,
		MsPerFrame:        32,
	})
}

func NewServiceWithOptions(v VAD, opt ServiceOptions) *Service {
	if opt.SpeechStartFrames == 0 {
		opt.SpeechStartFrames = 2
	}
	if opt.MsPerFrame == 0 {
		opt.MsPerFrame = 32
	}
	if opt.SilenceDurationMs == 0 {
		opt.SilenceDurationMs = 800
	}
	if opt.SilenceThreshold == 0 {
		opt.SilenceThreshold = 0.3
	}
	if opt.SpeechThreshold == 0 {
		opt.SpeechThreshold = 0.4
	}
	if opt.EnergyThreshold == 0 {
		opt.EnergyThreshold = 0.001
	}
	if opt.ResetAfterFrames == 0 {
		opt.ResetAfterFrames = 30 // ~ 1 second of silence — match Java
	}
	return &Service{
		vad:               v,
		speechTh:          opt.SpeechThreshold,
		silenceTh:         opt.SilenceThreshold,
		energyTh:          opt.EnergyThreshold,
		silenceFrames:     opt.SilenceDurationMs / opt.MsPerFrame,
		speechStartFrames: opt.SpeechStartFrames,
	}
}

// LastProb returns the most recent speech probability from the last Feed call.
// Exposed for diagnostics — production code reads the Status return only.
func (s *Service) LastProb() float32 { return s.lastProb }

// IsSpeaking reports whether the state machine is currently inside a
// SpeechStart..SpeechEnd window. Used by AudioPipeline to drive GRU resets.
func (s *Service) IsSpeaking() bool { return s.inSpeech }

// Feed runs the state machine on one PCM chunk for one stream. The supplied
// Session carries that stream's rolling context + LSTM state. When session is
// nil, falls back to the global VAD's stateless Process (legacy path).
func (s *Service) Feed(sess Session, pcm []int16) Status {
	var (
		prob float32
		err  error
	)
	if sess != nil {
		prob, err = sess.Process(pcm)
	} else {
		prob, err = s.vad.Process(pcm)
	}
	s.lastProb = prob
	s.lastErr = err
	if err != nil {
		return Error
	}

	// Optional energy gate: drop chunks with no audio energy regardless of
	// what the model says. Matches xiaozhi-java's energy-threshold filter
	// (silence + low-energy → never SpeechStart).
	hasEnergy := true
	if s.energyTh > 0 && len(pcm) > 0 {
		var absSum float64
		for _, x := range pcm {
			absSum += float64(absInt16(x))
		}
		mean := absSum / float64(len(pcm)) / 32768.0
		hasEnergy = mean > float64(s.energyTh)
	}

	if !s.inSpeech {
		// Need consecutiveSpeechFrames >= speechStartFrames before firing
		// SpeechStart (mirrors xiaozhi-java's de-bouncing).
		if prob >= s.speechTh && hasEnergy {
			s.speechCount++
			if s.speechCount >= s.speechStartFrames {
				s.inSpeech = true
				s.silenceCount = 0
				s.speechCount = 0
				return SpeechStart
			}
			return Silence
		}
		s.speechCount = 0
		return Silence
	}

	// Already in speech — keep going as long as probability is high.
	if prob >= s.speechTh {
		s.silenceCount = 0
		s.speechCount = 0
		return SpeechContinue
	}

	if s.silenceFrames == 0 {
		s.inSpeech = false
		s.speechCount = 0
		return SpeechEnd
	}

	if prob < s.silenceTh || !hasEnergy {
		s.silenceCount++
		if s.silenceCount >= s.silenceFrames {
			s.inSpeech = false
			s.silenceCount = 0
			s.speechCount = 0
			return SpeechEnd
		}
	} else {
		s.silenceCount = 0
	}
	return SpeechContinue
}

func absInt16(x int16) int32 {
	if x < 0 {
		return -int32(x)
	}
	return int32(x)
}

// Reset clears the state machine (does not touch any per-stream Session).
// Call this when a new dialogue turn starts and you want a clean transition.
func (s *Service) Reset() {
	s.inSpeech = false
	s.silenceCount = 0
	s.speechCount = 0
}