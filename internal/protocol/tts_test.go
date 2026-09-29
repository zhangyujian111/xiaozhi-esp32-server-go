package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTTSMessage_Unmarshal(t *testing.T) {
	data := `{"type":"tts","state":"start"}`
	var msg TTSMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, TTS, msg.Type)
	assert.Equal(t, TTSStateStart, msg.State)
	assert.Empty(t, msg.Text)
}

func TestTTSMessage_Unmarshal_SentenceStart(t *testing.T) {
	data := `{"type":"tts","state":"sentence_start","text":"今天天气真好"}`
	var msg TTSMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, TTSStateSentenceStart, msg.State)
	assert.Equal(t, "今天天气真好", msg.Text)
}

func TestTTSMessage_Unmarshal_Stop(t *testing.T) {
	data := `{"type":"tts","state":"stop"}`
	var msg TTSMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, TTSStateStop, msg.State)
}

func TestSTTMessage_Unmarshal(t *testing.T) {
	data := `{"type":"stt","text":"今天天气怎么样","emotion":"neutral"}`
	var msg STTMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, STT, msg.Type)
	assert.Equal(t, "今天天气怎么样", msg.Text)
	assert.Equal(t, "neutral", msg.Emotion)
}

func TestSTTMessage_RoundTrip(t *testing.T) {
	original := STTMessage{Type: STT, Text: "测试文本", Emotion: "happy"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded STTMessage
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	assert.Equal(t, original.Text, decoded.Text)
	assert.Equal(t, original.Emotion, decoded.Emotion)
}

func TestLLMMessage_Unmarshal(t *testing.T) {
	data := `{"type":"llm","emotion":"neutral","action":"","text":"您好，有什么可以帮助您的？"}`
	var msg LLMMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, LLM, msg.Type)
	assert.Equal(t, "neutral", msg.Emotion)
	assert.Equal(t, "", msg.Action)
	assert.Equal(t, "您好，有什么可以帮助您的？", msg.Text)
}

func TestLLMMessage_Unmarshal_WithAction(t *testing.T) {
	data := `{"type":"llm","emotion":"happy","action":"dance","text":"太棒了！"}`
	var msg LLMMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, "happy", msg.Emotion)
	assert.Equal(t, "dance", msg.Action)
	assert.Equal(t, "太棒了！", msg.Text)
}

func TestLLMMessage_RoundTrip(t *testing.T) {
	original := LLMMessage{Type: LLM, Emotion: "excited", Action: "wave", Text: "你好！"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded LLMMessage
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	assert.Equal(t, original.Emotion, decoded.Emotion)
	assert.Equal(t, original.Action, decoded.Action)
	assert.Equal(t, original.Text, decoded.Text)
}
