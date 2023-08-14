package resourcequota

import (
	"gpu-extend-controller/pkg/controllers/apis"
	"gpu-extend-controller/pkg/controllers/framework"
	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	v12 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	vcclientset "volcano.sh/apis/pkg/client/clientset/versioned"
	versionedscheme "volcano.sh/apis/pkg/client/clientset/versioned/scheme"
	informerfactory "volcano.sh/apis/pkg/client/informers/externalversions"
	vcinformer "volcano.sh/apis/pkg/client/informers/externalversions"
	schedulinginformer "volcano.sh/apis/pkg/client/informers/externalversions/scheduling/v1beta1"
	schedulinglister "volcano.sh/apis/pkg/client/listers/scheduling/v1beta1"
)

func Init() {
	framework.RegisterController(&resourceQuotaController{})
}

type resourceQuotaController struct {
	kubeClient kubernetes.Interface
	vcClient   vcclientset.Interface

	//informer
	queueInformer         schedulinginformer.QueueInformer
	resourceQuotaInformer v12.ResourceQuotaInformer

	// queueLister
	queueLister schedulinglister.QueueLister
	queueSynced cache.InformerSynced

	//resourceQuotaLister
	resourceQuotaLister corelisters.ResourceQuotaLister
	resourceQuotaSynced func() bool

	vcInformerFactory vcinformer.SharedInformerFactory
	informerfactory   informers.SharedInformerFactory

	//resourceQuota Event recorder
	recorder record.EventRecorder

	// queues that need to be updated.
	queue                workqueue.RateLimitingInterface
	enqueueResourceQuota func(req *apis.Request)
	syncHandler          func(req *apis.Request) error
	maxRequeueNum        int
}

func (r *resourceQuotaController) Name() string {
	return "resource-quota-controller"
}

// NewQueueController creates a QueueController.
func (r *resourceQuotaController) Initialize(opt *framework.ControllerOption) error {
	klog.Infof("Initialize for %s", r.Name())
	r.vcClient = opt.VolcanoClient
	r.kubeClient = opt.KubeClient

	factory := informerfactory.NewSharedInformerFactory(r.vcClient, 0)
	queueInformer := factory.Scheduling().V1beta1().Queues()

	sharedInformerFactory := opt.SharedInformerFactory
	resourceQuotaInformer := sharedInformerFactory.Core().V1().ResourceQuotas()

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: r.kubeClient.CoreV1().Events("")})

	r.queue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	r.informerfactory = sharedInformerFactory
	r.resourceQuotaInformer = resourceQuotaInformer
	r.resourceQuotaLister = resourceQuotaInformer.Lister()
	r.resourceQuotaSynced = resourceQuotaInformer.Informer().HasSynced

	r.vcInformerFactory = factory
	r.queueInformer = queueInformer
	r.queueLister = queueInformer.Lister()
	r.queueSynced = queueInformer.Informer().HasSynced

	r.recorder = eventBroadcaster.NewRecorder(versionedscheme.Scheme, v1.EventSource{Component: "gpu-extend-controller"})
	r.maxRequeueNum = opt.MaxRequeueNum
	if r.maxRequeueNum < 0 {
		r.maxRequeueNum = -1
	}

	//queueInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
	//	AddFunc:    r.addQueue,
	//	UpdateFunc: r.updateQueue,
	//	DeleteFunc: r.deleteQueue,
	//})

	resourceQuotaInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.addResourceQuota,
		UpdateFunc: r.updateResourceQuota,
		DeleteFunc: r.deleteResourceQuota,
	})

	r.syncHandler = r.handleResourceQuota
	r.enqueueResourceQuota = r.enqueue

	return nil
}

// Run starts QueueController.
func (r *resourceQuotaController) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	klog.Infof("Starting resourcequota controller.")
	defer klog.Infof("Shutting down resourcequota controller.")

	r.vcInformerFactory.Start(stopCh)
	r.informerfactory.Start(stopCh)

	for informerType, ok := range r.informerfactory.WaitForCacheSync(stopCh) {
		if !ok {
			klog.Errorf("caches failed to sync: %v", informerType)
			return
		}
	}

	for informerType, ok := range r.vcInformerFactory.WaitForCacheSync(stopCh) {
		if !ok {
			klog.Errorf("caches failed to sync: %v", informerType)
			return
		}
	}

	go wait.Until(r.worker, 0, stopCh)

	<-stopCh
}

// worker runs a worker thread that just dequeues items, processes them, and
// marks them done. You may run as many of these in parallel as you wish; the
// workqueue guarantees that they will not end up processing the same `queue`
// at the same time.
func (r *resourceQuotaController) worker() {
	for r.processNextWorkItem() {
	}
}

func (r *resourceQuotaController) processNextWorkItem() bool {
	obj, shutdown := r.queue.Get()
	if shutdown {
		return false
	}
	defer r.queue.Done(obj)

	req, ok := obj.(*apis.Request)
	if !ok {
		klog.Errorf("%v is not a valid resource quota request struct.", obj)
		return true
	}

	err := r.syncHandler(req)
	r.handleQueueErr(err, obj)

	return true
}