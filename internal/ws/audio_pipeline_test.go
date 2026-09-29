package ws

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type mockVADForPipeline struct {
	scores    []float32
	errs      []error
	callCount int
}

func (m *mockVADForPipeline) Process(pcm []int16) (float32, error) {
	s := m.scores[m.callCount]
	e := m.errs[m.callCount]
	m.callCount++
	return s, e
}

func TestAudioPipeline_SpeechStart_EmitsEvent(t *testing.T) {
	mock := &mockVADForPipeline{
		scores: []float32{0.9, 0.1, 0.1},
		errs:   []error{nil, nil, nil},
	}
	sm := NewSessionManager()
	p := NewAudioPipeline(sm, mock, 0.5, 0.3, 100)
	require.NotNil(t, p)

	sessionID := "test-session"
	session := NewChatSession("device1", sessionID)
	_ = sm.Register(session)

	p.Feed(sessionID, nil)

	require.Equal(t, 1, mock.callCount, "VAD should be called once")
}

func TestAudioPipeline_SpeechEnd_EmitsEvent(t *testing.T) {
	scores := []float32{0.9}
	for i := 0; i < 11; i++ {
		scores = append(scores, 0.1)
	}
	mock := &mockVADForPipeline{scores: scores, errs: make([]error, len(scores))}
	sm := NewSessionManager()
	p := NewAudioPipeline(sm, mock, 0.5, 0.3, 90)
	require.NotNil(t, p)

	sessionID := "test-session"
	session := NewChatSession("device1", sessionID)
	_ = sm.Register(session)

	p.Feed(sessionID, nil)
	for i := 0; i < 10; i++ {
		p.Feed(sessionID, nil)
	}

	require.Equal(t, 11, mock.callCount, "VAD should be called 11 times")
}

func TestAudioPipeline_UnknownSession_NoPanic(t *testing.T) {
	mock := &mockVADForPipeline{scores: []float32{0.1}, errs: []error{nil}}
	sm := NewSessionManager()
	p := NewAudioPipeline(sm, mock, 0.5, 0.3, 100)
	require.NotNil(t, p)

	require.NotPanics(t, func() {
		p.Feed("nonexistent", nil)
	})
}
