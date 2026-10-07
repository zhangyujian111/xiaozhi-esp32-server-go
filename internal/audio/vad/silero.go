//go:build silero

package vad

import (
	"errors"
	"fmt"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// SileroVAD runs the Silero VAD v5 ONNX model (16 kHz mono, 512-sample frames
// with 64-sample LSTM context prepended → effective input size 576).
//
// Model I/O contract (v5 unified):
//   input "input"  : float32[1, 576]    (64 context + 512 new samples, [-1, 1])
//   input "state"  : float32[2, 1, 128] (combined LSTM hidden+cell states: 2 layers × 1 batch × 128 hidden)
//   input "sr"     : int64[1]           (sample rate, must be 16000)
//   output "output": float32[1, 1]      (speech probability)
//   output "stateN": float32[2, 1, 128] (next LSTM state, same layout as state input)
//
// Each WebSocket stream gets its own *SileroSession holding the rolling
// 64-sample context and the 128-dim LSTM state. All sessions share the same
// AdvancedSession for inference (mutex-serialised).
type SileroVAD struct {
	mu sync.Mutex

	sess    *ort.AdvancedSession
	inPCM   *ort.Tensor[float32]
	inState *ort.Tensor[float32]
	inSR    *ort.Tensor[int64]

	outProb   *ort.Tensor[float32]
	outStateN *ort.Tensor[float32]
}

const (
	sileroWindowSamples = 512
	sileroContextSize  = 64
	sileroStateRows    = 2
	sileroStateDim     = 128
)

// NewSileroVAD constructs a SileroVAD from the Silero v5 ONNX file. The model
// is shared across all sessions (serialised by internal mutex). Each session
// should be opened via StartSession() and used independently.
func NewSileroVAD(modelPath string, sharedLibPath ...string) (*SileroVAD, error) {
	if sharedLibPath != nil && len(sharedLibPath) > 0 && sharedLibPath[0] != "" {
		ort.SetSharedLibraryPath(sharedLibPath[0])
	}
	if !ort.IsInitialized() {
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, fmt.Errorf("init onnx runtime: %w", err)
		}
	}

	inPCM, err := ort.NewTensor[float32](ort.NewShape(1, sileroWindowSamples+sileroContextSize), make([]float32, sileroWindowSamples+sileroContextSize))
	if err != nil {
		return nil, fmt.Errorf("alloc input pcm: %w", err)
	}
	// State shape MUST be (2, 1, 128): 2 LSTM layers, batch=1, hidden=128.
	// xiaozhi-java uses the same layout. See SileroVadModel.infer in the Java
	// reference implementation.
	inState, err := ort.NewTensor[float32](ort.NewShape(sileroStateRows, 1, sileroStateDim), make([]float32, sileroStateRows*sileroStateDim))
	if err != nil {
		return nil, fmt.Errorf("alloc input state: %w", err)
	}
	inSR, err := ort.NewTensor[int64](ort.NewShape(1), []int64{16000})
	if err != nil {
		return nil, fmt.Errorf("alloc input sr: %w", err)
	}
	outProb, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 1))
	if err != nil {
		return nil, fmt.Errorf("alloc output prob: %w", err)
	}
	outStateN, err := ort.NewEmptyTensor[float32](ort.NewShape(sileroStateRows, 1, sileroStateDim))
	if err != nil {
		return nil, fmt.Errorf("alloc output stateN: %w", err)
	}

	sess, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"input", "state", "sr"},
		[]string{"output", "stateN"},
		[]ort.Value{inPCM, inState, inSR},
		[]ort.Value{outProb, outStateN},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("load onnx model %s: %w", modelPath, err)
	}
	return &SileroVAD{
		sess:      sess,
		inPCM:     inPCM,
		inState:   inState,
		inSR:      inSR,
		outProb:   outProb,
		outStateN: outStateN,
	}, nil
}

// MintSession returns a new Session. Implements SessionFactory.
func (v *SileroVAD) MintSession() Session {
	return &sileroSession{parent: v, context: make([]float32, sileroContextSize), state: make([]float32, sileroStateRows*sileroStateDim)}
}

// StartSession returns a fresh per-stream session.
func (v *SileroVAD) StartSession() Session {
	return v.MintSession()
}

// Process on the shared VAD runs inference with no per-stream state.
// Stateful VAD requires Session.Process — this method returns an error
// to flag the misuse; callers should mint a Session via StartSession().
func (v *SileroVAD) Process(pcm []int16) (float32, error) {
	return 0, errors.New("silero VAD requires per-session state; call Session.Process instead")
}

type sileroSession struct {
	parent  *SileroVAD
	mu      sync.Mutex
	context []float32
	state   []float32
}

func (s *sileroSession) Process(pcm []int16) (float32, error) {
	if s == nil || s.parent == nil {
		return 0, errors.New("silero VAD session not initialised")
	}
	if len(pcm) == 0 {
		return 0, errors.New("empty pcm buffer")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	v := s.parent
	v.mu.Lock()
	defer v.mu.Unlock()

	// Build the 576-sample input as {context[64] + samples[512]}, int16 → float32 in [-1, 1].
	dstPCM := v.inPCM.GetData()
	for i := 0; i < sileroContextSize; i++ {
		dstPCM[i] = s.context[i]
	}
	for i := 0; i < sileroWindowSamples; i++ {
		if i < len(pcm) {
			dstPCM[sileroContextSize+i] = float32(pcm[i]) / 32768.0
		} else {
			dstPCM[sileroContextSize+i] = 0
		}
	}

	// Roll current LSTM state into the input tensor (state is [1, 2, 128]).
	dstState := v.inState.GetData()
	for i := range dstState {
		dstState[i] = s.state[i]
	}

	if err := v.sess.Run(); err != nil {
		return 0, fmt.Errorf("onnx run: %w", err)
	}

	prob := v.outProb.GetData()[0]
	newState := v.outStateN.GetData()
	for i := range newState {
		s.state[i] = newState[i]
	}

	// Update context: keep the last 64 samples of the current frame.
	for i := 0; i < sileroContextSize; i++ {
		idx := len(pcm) - sileroContextSize + i
		switch {
		case idx < 0, idx >= len(pcm):
			s.context[i] = 0
		default:
			s.context[i] = float32(pcm[idx]) / 32768.0
		}
	}

	return prob, nil
}

// Reset zeroes the per-session LSTM hidden state and the 64-sample rolling
// context. Used by AudioPipeline after long silence to avoid GRU state
// drift (mirrors xiaozhi-java's SILENCE_RESET_FRAMES behaviour).
func (s *sileroSession) Reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.context {
		s.context[i] = 0
	}
	for i := range s.state {
		s.state[i] = 0
	}
}

// Destroy releases native resources. Safe to call multiple times.
func (v *SileroVAD) Destroy() {
	if v == nil || v.sess == nil {
		return
	}
	v.sess.Destroy()
	v.sess = nil
}