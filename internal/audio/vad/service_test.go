package vad

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockVAD struct {
	scores []float32
	errs   []error
	idx    int
}

func (m *mockVAD) Process(pcm []int16) (float32, error) {
	s := m.scores[m.idx]
	e := m.errs[m.idx]
	m.idx++
	return s, e
}

func TestService_SilenceToSpeechStart(t *testing.T) {
	mock := &mockVAD{scores: []float32{0.9, 0.1, 0.1, 0.1}, errs: []error{nil, nil, nil, nil}}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	status := svc.Feed(nil)
	require.Equal(t, SpeechStart, status, "should transition to speech when score exceeds threshold")
}

func TestService_SpeechToEnd(t *testing.T) {
	scores := []float32{0.9}
	for i := 0; i < 11; i++ {
		scores = append(scores, 0.1)
	}
	mock := &mockVAD{scores: scores, errs: make([]error, len(scores))}
	svc := NewService(mock, 0.5, 0.3, 90)
	require.NotNil(t, svc)

	_ = svc.Feed(nil)
	var lastStatus Status
	for i := 0; i < 9; i++ {
		lastStatus = svc.Feed(nil)
	}
	require.Equal(t, SpeechEnd, lastStatus, "should end speech after silence frames")
}

func TestService_SpeechContinue(t *testing.T) {
	mock := &mockVAD{scores: []float32{0.9, 0.8, 0.7, 0.6, 0.5}, errs: make([]error, 5)}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	_ = svc.Feed(nil)
	for i := 1; i < 4; i++ {
		status := svc.Feed(nil)
		require.Equal(t, SpeechContinue, status)
	}
}

func TestService_VADError(t *testing.T) {
	testErr := errors.New("vad error")
	mock := &mockVAD{scores: []float32{0.1}, errs: []error{testErr}}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	status := svc.Feed(nil)
	require.Equal(t, Error, status, "should return Error when VAD returns error")
}

func TestService_SpeechAfterSilenceThreshold(t *testing.T) {
	mock := &mockVAD{scores: []float32{0.9, 0.4, 0.4, 0.4, 0.4}, errs: make([]error, 5)}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	_ = svc.Feed(nil)
	for i := 1; i < 4; i++ {
		status := svc.Feed(nil)
		require.Equal(t, SpeechContinue, status)
	}
}

func TestService_ZeroSilenceFrames(t *testing.T) {
	mock := &mockVAD{scores: []float32{0.9, 0.1}, errs: []error{nil, nil}}
	svc := NewService(mock, 0.5, 0.3, 0)
	require.NotNil(t, svc)

	_ = svc.Feed(nil)
	status := svc.Feed(nil)
	require.Equal(t, SpeechEnd, status, "zero silence frames should immediately end speech")
}

func TestService_SpeechMidThreshold(t *testing.T) {
	mock := &mockVAD{scores: []float32{0.9, 0.4, 0.4, 0.4}, errs: []error{nil, nil, nil, nil}}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	_ = svc.Feed(nil)
	status := svc.Feed(nil)
	require.Equal(t, SpeechContinue, status, "mid threshold during speech resets silence count")
}

func TestService_SpeechContinue_StaysAboveThreshold(t *testing.T) {
	mock := &mockVAD{scores: []float32{0.9, 0.9, 0.9, 0.9}, errs: []error{nil, nil, nil, nil}}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	_ = svc.Feed(nil)
	for i := 0; i < 3; i++ {
		status := svc.Feed(nil)
		require.Equal(t, SpeechContinue, status)
	}
}

func TestService_SpeechContinue_ResetSilenceCount(t *testing.T) {
	mock := &mockVAD{scores: []float32{0.9, 0.1, 0.4, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1}, errs: make([]error, 10)}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	_ = svc.Feed(nil)
	_ = svc.Feed(nil)
	status := svc.Feed(nil)
	require.Equal(t, SpeechContinue, status)
}
