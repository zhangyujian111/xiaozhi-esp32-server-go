//go:build silero

package vad

import (
	"errors"
	"fmt"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// SileroVAD runs the Silero VAD v4 ONNX model (16 kHz mono, 512-sample frames).
//
// Model I/O contract (v4):
//   input "input" : float32[1, 512]
//   input "h"     : float32[1, 1, 64]
//   input "c"     : float32[1, 1, 64]
//   output "output" : float32[1, 1]
//   output "hn"   : float32[1, 1, 64]
//   output "cn"   : float32[1, 1, 64]
//
// Onnxruntime DLL is loaded from onnxSharedLibraryPath (set by NewSileroVAD
// or via the default search path). Model weights are loaded from disk.
type SileroVAD struct {
	mu     sync.Mutex
	sess   *ort.AdvancedSession
	inPCM  *ort.Tensor[float32]
	inH    *ort.Tensor[float32]
	inC    *ort.Tensor[float32]
	outProb *ort.Tensor[float32]
	outHN  *ort.Tensor[float32]
	outCN  *ort.Tensor[float32]
}

// NewSileroVAD constructs a SileroVAD from an ONNX file. sharedLibPath is the
// absolute path to onnxruntime.dll (or empty to use the default search path).
func NewSileroVAD(modelPath string, sharedLibPath ...string) (*SileroVAD, error) {
	if sharedLibPath != nil && len(sharedLibPath) > 0 && sharedLibPath[0] != "" {
		ort.SetSharedLibraryPath(sharedLibPath[0])
	}
	if !ort.IsInitialized() {
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, fmt.Errorf("init onnx runtime: %w", err)
		}
	}

	inPCM, err := ort.NewTensor[float32](ort.NewShape(1, 512), make([]float32, 512))
	if err != nil {
		return nil, fmt.Errorf("alloc input pcm: %w", err)
	}
	inH, err := ort.NewTensor[float32](ort.NewShape(1, 1, 64), make([]float32, 64))
	if err != nil {
		return nil, fmt.Errorf("alloc input h: %w", err)
	}
	inC, err := ort.NewTensor[float32](ort.NewShape(1, 1, 64), make([]float32, 64))
	if err != nil {
		return nil, fmt.Errorf("alloc input c: %w", err)
	}
	outProb, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 1))
	if err != nil {
		return nil, fmt.Errorf("alloc output prob: %w", err)
	}
	outHN, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 1, 64))
	if err != nil {
		return nil, fmt.Errorf("alloc output hn: %w", err)
	}
	outCN, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 1, 64))
	if err != nil {
		return nil, fmt.Errorf("alloc output cn: %w", err)
	}

	sess, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"input", "h", "c"},
		[]string{"output", "hn", "cn"},
		[]ort.Value{inPCM, inH, inC},
		[]ort.Value{outProb, outHN, outCN},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("load onnx model %s: %w", modelPath, err)
	}
	return &SileroVAD{
		sess:    sess,
		inPCM:   inPCM,
		inH:     inH,
		inC:     inC,
		outProb: outProb,
		outHN:   outHN,
		outCN:   outCN,
	}, nil
}

// Process runs inference on a 512-sample int16 PCM frame (16 kHz mono, 32 ms).
// Returns speech probability in [0, 1]. pcm must be exactly 512 samples; if
// shorter, it is zero-padded; if longer, the trailing samples are dropped.
func (v *SileroVAD) Process(pcm []int16) (float32, error) {
	if v == nil || v.sess == nil {
		return 0, errors.New("silero VAD not initialized")
	}
	if len(pcm) == 0 {
		return 0, errors.New("empty pcm buffer")
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	// Convert int16 → float32 in [-1, 1] into the pre-allocated input buffer.
	dst := v.inPCM.GetData()
	for i := 0; i < 512; i++ {
		if i < len(pcm) {
			dst[i] = float32(pcm[i]) / 32768.0
		} else {
			dst[i] = 0
		}
	}

	if err := v.sess.Run(); err != nil {
		return 0, fmt.Errorf("onnx run: %w", err)
	}

	prob := v.outProb.GetData()[0]

	// Roll LSTM state: copy hn/cn back into h/c for the next frame.
	newH := v.outHN.GetData()
	hBuf := v.inH.GetData()
	for i := range newH {
		hBuf[i] = newH[i]
	}
	newC := v.outCN.GetData()
	cBuf := v.inC.GetData()
	for i := range newC {
		cBuf[i] = newC[i]
	}

	return prob, nil
}

// Destroy releases native resources. Safe to call multiple times.
func (v *SileroVAD) Destroy() {
	if v == nil || v.sess == nil {
		return
	}
	v.sess.Destroy()
	v.sess = nil
}