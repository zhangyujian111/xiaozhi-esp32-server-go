package dialogue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInterruptController_SetCancel_Trigger(t *testing.T) {
	ic := NewInterruptController()
	ctx, cancel := context.WithCancel(context.Background())
	ic.SetCancel(cancel)

	require.False(t, ic.Triggered())

	ic.Trigger()

	require.True(t, ic.Triggered())

	select {
	case <-ctx.Done():
	default:
		t.Fatal("context should be canceled after Trigger")
	}
}

func TestInterruptController_NilCancel_NoPanic(t *testing.T) {
	ic := NewInterruptController()
	ic.SetCancel(nil)
	require.NotPanics(t, func() { ic.Trigger() })
	require.True(t, ic.Triggered())
}

func TestInterruptController_Triggered(t *testing.T) {
	ic := NewInterruptController()
	require.False(t, ic.Triggered())

	ic.Trigger()
	require.True(t, ic.Triggered())
}

func TestInterruptController_ReSetCancel(t *testing.T) {
	ic := NewInterruptController()

	_, cancel1 := context.WithCancel(context.Background())
	ic.SetCancel(cancel1)

	ctx2, cancel2 := context.WithCancel(context.Background())
	ic.SetCancel(cancel2)

	ic.Trigger()

	require.True(t, ic.Triggered())

	select {
	case <-ctx2.Done():
	default:
		t.Fatal("latest context should be canceled after Trigger")
	}
}
