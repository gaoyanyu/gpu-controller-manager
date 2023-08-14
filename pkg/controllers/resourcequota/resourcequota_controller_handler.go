package resourcequota

import (
	"context"
	"errors"
	"fmt"
	"gpu-extend-controller/pkg/controllers/apis"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	"strings"
	"time"
	"volcano.sh/apis/pkg/apis/scheduling/v1beta1"
)

const (
	AddAction    = "Add"
	UpdateAction = "Update"
	DeleteAction = "Delete"
)

func (r *resourceQuotaController) enqueue(req *apis.Request) {
	r.queue.Add(req)
}

func (r *resourceQuotaController) addResourceQuota(obj interface{}) {
	quota := obj.(*v1.ResourceQuota)
	//if strings.HasPrefix(quota.Name, "infra-3090") {
	//	return
	//}
	if strings.HasSuffix(quota.Namespace, "3090") ||
		strings.HasSuffix(quota.Namespace, "a100") ||
		strings.HasSuffix(quota.Name, "3090") ||
		strings.HasSuffix(quota.Name, "a100") {
		req := &apis.Request{
			ResourceQuota: quota,
			Verb: AddAction,
		}
		r.enqueue(req)
	}
}

func (r *resourceQuotaController) updateResourceQuota(oldObj, newObj interface{}) {
	newQuota := newObj.(*v1.ResourceQuota)
	//if strings.HasPrefix(newQuota.Name, "infra-3090") {
	//	return
	//}
	if strings.HasSuffix(newQuota.Namespace, "3090") ||
		strings.HasSuffix(newQuota.Namespace, "a100") ||
		strings.HasSuffix(newQuota.Name, "3090") ||
		strings.HasSuffix(newQuota.Name, "a100") {
		req := &apis.Request{
			ResourceQuota: newQuota,
			Verb: UpdateAction,
		}
		r.enqueue(req)
	}
}

func (r *resourceQuotaController) deleteResourceQuota(obj interface{}) {
	quota := obj.(*v1.ResourceQuota)
	//if strings.HasPrefix(quota.Name, "infra-3090") {
	//	return
	//}
	if strings.HasSuffix(quota.Namespace, "3090") ||
		strings.HasSuffix(quota.Namespace, "a100") ||
		strings.HasSuffix(quota.Name, "3090") ||
		strings.HasSuffix(quota.Name, "a100") {
		req := &apis.Request{
			ResourceQuota: quota,
			Verb: DeleteAction,
		}
		r.enqueue(req)
	}
}

func (r *resourceQuotaController) handleResourceQuota(req *apis.Request) error {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing resource quota %s (%v).", req.ResourceQuota.Name, time.Since(startTime))
	}()

	switch req.Verb {
	case AddAction:
		return r.handleResourceQuotaAction(req)
	case UpdateAction:
		return r.handleResourceQuotaAction(req)
	case DeleteAction:
		return r.handleResourceQuotaAction(req)
	default:
		return errors.New("unKnow req verb")
	}
}

func (r *resourceQuotaController) handleResourceQuotaAction(req *apis.Request) error {
	queue, err := r.queueLister.Get(req.ResourceQuota.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Infof("Queue %s not found", req.ResourceQuota.Name)
			if req.Verb == DeleteAction {
				return nil
			}
		} else {
			klog.Errorf("get queue err when add: %s", err.Error())
			return err
		}
	}
	if queue != nil {
		if req.Verb == DeleteAction {
			err := r.vcClient.SchedulingV1beta1().Queues().Delete(context.Background(), queue.Name, metav1.DeleteOptions{})
			if err != nil {
				klog.Errorf("delete queue failed: %v", err)
				return err
			}
			klog.Errorf("delete queue %v success", queue.Name)
			return nil
		}
		queueCopy := queue.DeepCopy()

		resourceQuotaCpu := req.ResourceQuota.Spec.Hard["limits.cpu"]
		queueQuantityCpu := queueCopy.Spec.Capability[v1.ResourceCPU]
		if queueQuantityCpu.Value() != resourceQuotaCpu.Value() {
			queue.Spec.Capability[v1.ResourceCPU] = resourceQuotaCpu
		}

		resourceQuotaMem := req.ResourceQuota.Spec.Hard["limits.memory"]
		queueQuantityMem := queueCopy.Spec.Capability[v1.ResourceMemory]
		if queueQuantityMem.Value() != resourceQuotaMem.Value() {
			queue.Spec.Capability[v1.ResourceMemory] = resourceQuotaMem
		}

		resourceQuotaGpu := req.ResourceQuota.Spec.Hard["requests.nvidia.com/gpu"]
		queueQuantityGpu := queueCopy.Spec.Capability["nvidia.com/gpu"]
		if queueQuantityGpu.Value() != resourceQuotaGpu.Value() {
			queue.Spec.Capability["nvidia.com/gpu"] = resourceQuotaGpu
		}

		reclaimable := false
		queue.Spec.Reclaimable = &reclaimable
		if queue.ObjectMeta.Labels == nil {
			queue.ObjectMeta.Labels = make(map[string]string)
			queue.ObjectMeta.Labels["updateAt"] = time.Now().Format("2006-01-02-15-04-05")
		} else {
			queue.ObjectMeta.Labels["updateAt"] = time.Now().Format("2006-01-02-15-04-05")
		}
		_, err := r.vcClient.SchedulingV1beta1().Queues().Update(context.Background(), queue, metav1.UpdateOptions{})
		if err != nil {
			klog.Errorf("update queue failed: %v", err)
			return err
		}
		klog.Infof("update queue %s success", queue.Name)
		return nil
	}
	resourceList := make(map[v1.ResourceName]resource.Quantity)
	resourceList[v1.ResourceCPU] = req.ResourceQuota.Spec.Hard["limits.cpu"]
	resourceList[v1.ResourceMemory] = req.ResourceQuota.Spec.Hard["limits.memory"]
	resourceList["nvidia.com/gpu"] = req.ResourceQuota.Spec.Hard["requests.nvidia.com/gpu"]
	reclaimable := false
	targetQueue := &v1beta1.Queue{
		ObjectMeta: metav1.ObjectMeta{Name: req.ResourceQuota.Name},
		Spec: v1beta1.QueueSpec{
			Weight: 1,
			Capability: resourceList,
			Reclaimable: &reclaimable,
		},
	}

	respQueue, err := r.vcClient.SchedulingV1beta1().Queues().Create(context.Background(), targetQueue, metav1.CreateOptions{})
	if err != nil {
		klog.Errorf("create queue failed: %v", err)
		return err
	}
	klog.Infof("create queue %s success", respQueue.Name)
	return nil
}

func (r *resourceQuotaController) handleQueueErr(err error, obj interface{}) {
	if err == nil {
		r.queue.Forget(obj)
		return
	}

	if r.maxRequeueNum == -1 || r.queue.NumRequeues(obj) < r.maxRequeueNum {
		klog.V(4).Infof("Error syncing resource quota request %v for %v.", obj, err)
		r.queue.AddRateLimited(obj)
		return
	}

	req, _ := obj.(*apis.Request)
	r.recorder.Event(req.ResourceQuota, v1.EventTypeWarning,
		fmt.Sprintf("%v queue for resource quota %s", req.Verb, req.ResourceQuota.Name),
		fmt.Sprintf("%v queue for resource quota %s failed for %v", req.Verb, req.ResourceQuota.Name, err))

	klog.V(2).Infof("Dropping resource quota request %v out of the queue for %v.", obj, err)
	r.queue.Forget(obj)
}