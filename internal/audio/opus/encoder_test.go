package opus

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpusEncoder_NotImplementedReturnsError(t *testing.T) {
	enc, err := NewEncoder(24000, 1, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported", "encoder should return not supported error")
	require.Nil(t, enc)
}
