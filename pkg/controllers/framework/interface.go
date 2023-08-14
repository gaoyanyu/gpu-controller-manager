package framework

import (
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	vcclientset "volcano.sh/apis/pkg/client/clientset/versioned"
)

// ControllerOption is the main context object for the controllers.
type ControllerOption struct {
	KubeClient            kubernetes.Interface
	VolcanoClient         vcclientset.Interface
	SharedInformerFactory informers.SharedInformerFactory
	SchedulerNames        []string
	WorkerNum             uint32
	MaxRequeueNum         int

	InheritOwnerAnnotations bool
}

// Controller is the interface of all controllers.
type Controller interface {
	Name() string
	Initialize(opt *ControllerOption) error
	// Run run the controller
	Run(stopCh <-chan struct{})
}