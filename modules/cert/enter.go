package cert

import (
	"github.com/chenggegedeid/catflap/modules/cert/model"
	"sync"
)

type CertRuntime struct {
	mu        sync.RWMutex
	Config    model.CertConfig
	Manager   *Manager
	Scheduler *Scheduler
}

var Runtime = &CertRuntime{
	Manager: NewManager(),
}
