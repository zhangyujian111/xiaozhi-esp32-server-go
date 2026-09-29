package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMustMarshal(t *testing.T) {
	data := MustMarshal(HelloMessage{Type: Hello})
	assert.Contains(t, string(data), `"type":"hello"`)
}

func TestMustMarshal_Panic(t *testing.T) {
	assert.Panics(t, func() {
		MustMarshal(make(chan any))
	})
}
