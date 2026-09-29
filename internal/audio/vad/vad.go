package vad

type Status int

const (
	Silence Status = iota
	SpeechStart
	SpeechContinue
	SpeechEnd
	Error
)

type VAD interface {
	Process(pcm []int16) (probability float32, err error)
}

type Service struct {
	vad           VAD
	speechTh      float32
	silenceTh     float32
	silenceFrames int
	inSpeech      bool
	silenceCount  int
}

func NewService(v VAD, speechTh, silenceTh float32, silenceMs int) *Service {
	return &Service{
		vad:           v,
		speechTh:      speechTh,
		silenceTh:     silenceTh,
		silenceFrames: silenceMs / 10,
	}
}

func (s *Service) Feed(pcm []int16) Status {
	prob, err := s.vad.Process(pcm)
	if err != nil {
		return Error
	}

	if !s.inSpeech {
		if prob >= s.speechTh {
			s.inSpeech = true
			s.silenceCount = 0
			return SpeechStart
		}
		return Silence
	}

	if prob >= s.speechTh {
		s.silenceCount = 0
		return SpeechContinue
	}

	if s.silenceFrames == 0 {
		s.inSpeech = false
		return SpeechEnd
	}

	if prob < s.silenceTh {
		s.silenceCount++
		if s.silenceCount >= s.silenceFrames {
			s.inSpeech = false
			s.silenceCount = 0
			return SpeechEnd
		}
	} else {
		s.silenceCount = 0
	}
	return SpeechContinue
}
