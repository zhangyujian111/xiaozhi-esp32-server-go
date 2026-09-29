//go:build !nolong

package protocol

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Fixture struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	InputHex    string          `json:"input_hex"`
	Expected    json.RawMessage `json:"expected"`
	ExpectError bool            `json:"expect_error"`
}

func loadFixtures(t *testing.T) []*Fixture {
	// Dynamically locate fixtures directory relative to test file
	testFile := "."
	for i := 0; i < 6; i++ {
		dir := filepath.Join(testFile, "test", "conformance", "fixtures", "protocol")
		if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
			files, err := filepath.Glob(filepath.Join(dir, "*.json"))
			require.NoError(t, err)
			fixtures := make([]*Fixture, 0, len(files))
			for _, f := range files {
				data, err := os.ReadFile(f)
				require.NoError(t, err)
				var fixt Fixture
				require.NoError(t, json.Unmarshal(data, &fixt))
				fixtures = append(fixtures, &fixt)
			}
			return fixtures
		}
		testFile = filepath.Join(testFile, "..")
	}
	t.Skip("fixtures directory not found, skipping")
	return nil
}

func TestFixture_Hello(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "hello" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				if f.ExpectError {
					var msg HelloMessage
					err := json.Unmarshal(data, &msg)
					assert.Error(t, err, "expected unmarshal error")
				} else {
					var msg HelloMessage
					err := json.Unmarshal(data, &msg)
					assert.NoError(t, err)
					assert.Equal(t, Hello, msg.Type)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 5, "expected at least 5 hello fixtures")
}

func TestFixture_Listen(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "listen" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				if f.ExpectError {
					var msg ListenMessage
					err := json.Unmarshal(data, &msg)
					assert.Error(t, err)
				} else {
					var msg ListenMessage
					err := json.Unmarshal(data, &msg)
					assert.NoError(t, err)
					assert.Equal(t, Listen, msg.Type)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 10)
}

func TestFixture_Abort(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "abort" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				var msg AbortMessage
				err = json.Unmarshal(data, &msg)
				if f.ExpectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, Abort, msg.Type)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 3)
}

func TestFixture_Ack(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "ack" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				var msg AckMessage
				err = json.Unmarshal(data, &msg)
				if f.ExpectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, Ack, msg.Type)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 5)
}

func TestFixture_TTS(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "tts" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				var msg TTSMessage
				err = json.Unmarshal(data, &msg)
				if f.ExpectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, TTS, msg.Type)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 8)
}

func TestFixture_STT(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "stt" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				var msg STTMessage
				err = json.Unmarshal(data, &msg)
				if f.ExpectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, STT, msg.Type)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 5)
}

func TestFixture_LLM(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "llm" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				var msg LLMMessage
				err = json.Unmarshal(data, &msg)
				if f.ExpectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, LLM, msg.Type)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 5)
}

func TestFixture_MCP(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "mcp" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				var msg MCPMessage
				err = json.Unmarshal(data, &msg)
				if f.ExpectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, MCP, msg.Type)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 10)
}

func TestFixture_IoT(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "iot" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				var msg IoTMessage
				err = json.Unmarshal(data, &msg)
				if f.ExpectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, IoT, msg.Type)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 5)
}

func TestFixture_Alert(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "alert" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				var msg AlertMessage
				err = json.Unmarshal(data, &msg)
				if f.ExpectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, Alert, msg.Type)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 3)
}

func TestFixture_System(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "system" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				var msg SystemMessage
				err = json.Unmarshal(data, &msg)
				if f.ExpectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, System, msg.Type)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 3)
}

func TestFixture_BinaryV1(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "binary_v1" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				_, _, _, err = ParseBinaryFrame(data, 1)
				if f.ExpectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 5)
}

func TestFixture_BinaryV2(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "binary_v2" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				_, _, _, err = ParseBinaryFrame(data, 2)
				if f.ExpectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 15)
}

func TestFixture_BinaryV3(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "binary_v3" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				_, _, _, err = ParseBinaryFrame(data, 3)
				if f.ExpectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	}
	assert.GreaterOrEqual(t, count, 15)
}

func TestFixture_Malformed(t *testing.T) {
	fixtures := loadFixtures(t)
	var count int
	for _, f := range fixtures {
		if f.Type == "error" {
			count++
			t.Run(f.Name, func(t *testing.T) {
				data, err := hex.DecodeString(f.InputHex)
				require.NoError(t, err)
				var raw json.RawMessage
				err = json.Unmarshal(data, &raw)
				assert.NoError(t, err, "malformed fixtures must be valid JSON")
				assert.NotNil(t, raw, "raw message should be present")
			})
		}
	}
	assert.GreaterOrEqual(t, count, 8)
}

func TestFixture_TotalCount(t *testing.T) {
	fixtures := loadFixtures(t)
	assert.GreaterOrEqual(t, len(fixtures), 100, "expected at least 100 fixtures")
}
