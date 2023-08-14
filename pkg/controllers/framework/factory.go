package framework

import (
	"fmt"
	"k8s.io/klog"
)

var controllers = map[string]Controller{}

// ForeachController is helper function to operator all controllers.
func ForeachController(fn func(controller Controller)) {
	for _, ctrl := range controllers {
		fn(ctrl)
	}
}

// RegisterController register controller to the controller manager.
func RegisterController(ctrl Controller) error {
	if ctrl == nil {
		return fmt.Errorf("controller is nil")
	}

	if _, found := controllers[ctrl.Name()]; found {
		return fmt.Errorf("duplicated controller")
	}

	klog.V(3).Infof("Controller <%s> is registered.", ctrl.Name())
	controllers[ctrl.Name()] = ctrl
	return nil
}
