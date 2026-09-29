package dialogue

import (
	"context"
	"sync"
)

type InterruptController struct {
	mu        sync.Mutex
	cancelFn  context.CancelFunc
	triggered bool
}

func NewInterruptController() *InterruptController {
	return &InterruptController{}
}

func (ic *InterruptController) SetCancel(cancel context.CancelFunc) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.cancelFn = cancel
}

func (ic *InterruptController) Trigger() {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.triggered = true
	if ic.cancelFn != nil {
		ic.cancelFn()
	}
}

func (ic *InterruptController) Triggered() bool {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	return ic.triggered
}
