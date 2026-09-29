package dialogue

import (
	"strings"
)

type SentenceSplitter struct {
	buffer strings.Builder
}

func NewSentenceSplitter() *SentenceSplitter {
	return &SentenceSplitter{}
}

var delimiters = []string{"。", "！", "？", "?", "!", "；", ";", "\n"}

func (s *SentenceSplitter) Feed(token string) []string {
	s.buffer.WriteString(token)
	text := s.buffer.String()

	runes := []rune(text)
	lastRuneIdx := -1
	for _, d := range delimiters {
		drunes := []rune(d)
		for i := len(runes) - len(drunes); i >= 0; i-- {
			if string(runes[i:i+len(drunes)]) == d {
				if i > lastRuneIdx {
					lastRuneIdx = i
				}
				break
			}
		}
	}
	if lastRuneIdx < 0 {
		return []string{}
	}

	out := string(runes[:lastRuneIdx+1])
	s.buffer.Reset()
	s.buffer.WriteString(string(runes[lastRuneIdx+1:]))
	return []string{out}
}

func (s *SentenceSplitter) Flush() string {
	rest := s.buffer.String()
	s.buffer.Reset()
	return rest
}
