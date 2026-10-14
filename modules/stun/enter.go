package stun

import "github.com/chenggegedeid/catflap/modules/stun/model"

type STUNRuntime struct {
	Config      model.Config
	Network     model.NetworkState
	STUNService *STUNService
	UpnpGateway *model.UpnpGateway
	Scheduler   *Scheduler
}

var Runtime = &STUNRuntime{}
