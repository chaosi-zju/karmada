/*
Copyright 2024 The Karmada Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workloadrebalancer

import (
	"context"
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	appsv1alpha1 "github.com/karmada-io/karmada/pkg/apis/apps/v1alpha1"
	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	"github.com/karmada-io/karmada/pkg/util"
	"github.com/karmada-io/karmada/pkg/util/names"
)

const (
	// ControllerName is the controller name that will be used when reporting events.
	ControllerName = "workload-rebalancer"

	ttlAfterFinishedWorkerNum = 1
)

// RebalancerController is to handle a rebalance to workloads selected by WorkloadRebalancer object.
type RebalancerController struct {
	Client             client.Client // used to operate WorkloadRebalancer resources from cache.
	ControlPlaneClient client.Client // used to fetch arbitrary resources from api server.

	ttlAfterFinishedWorker util.AsyncWorker
}

// SetupWithManager creates a controller and register to controller manager.
func (c *RebalancerController) SetupWithManager(mgr controllerruntime.Manager) error {
	var predicateFunc = predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			c.ttlAfterFinishedWorker.Add(client.ObjectKey{Name: e.Object.GetName()})
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj := e.ObjectOld.(*appsv1alpha1.WorkloadRebalancer)
			newObj := e.ObjectNew.(*appsv1alpha1.WorkloadRebalancer)
			c.ttlAfterFinishedWorker.Add(client.ObjectKey{Name: newObj.GetName()})
			return !reflect.DeepEqual(oldObj.Spec, newObj.Spec)
		},
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}

	ttlAfterFinishedWorkerOptions := util.Options{
		Name:          "ttl-after-finished-worker",
		ReconcileFunc: c.deleteExpiredRebalancer,
	}
	c.ttlAfterFinishedWorker = util.NewAsyncWorker(ttlAfterFinishedWorkerOptions)
	c.ttlAfterFinishedWorker.Run(ttlAfterFinishedWorkerNum, context.Background().Done())

	return controllerruntime.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&appsv1alpha1.WorkloadRebalancer{}, builder.WithPredicates(predicateFunc)).
		Complete(c)
}

// Reconcile performs a full reconciliation for the object referred to by the Request.
// The Controller will requeue the Request to be processed again if an error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (c *RebalancerController) Reconcile(ctx context.Context, req controllerruntime.Request) (controllerruntime.Result, error) {
	klog.V(4).Infof("Reconciling for WorkloadRebalancer %s", req.Name)

	// 1. get latest WorkloadRebalancer
	oldRebalancer := &appsv1alpha1.WorkloadRebalancer{}
	if err := c.Client.Get(ctx, req.NamespacedName, oldRebalancer); err != nil {
		if apierrors.IsNotFound(err) {
			klog.Infof("no need to reconcile WorkloadRebalancer for it not found")
			return controllerruntime.Result{}, nil
		}
		return controllerruntime.Result{}, err
	}
	rebalancer := oldRebalancer.DeepCopy()

	// 2. get and update referenced binding to trigger a rescheduling
	successNum, retryNum := c.doWorkloadRebalance(ctx, rebalancer)

	// 3. update status of WorkloadRebalancer
	if err := c.updateWorkloadRebalancerStatus(ctx, oldRebalancer, rebalancer); err != nil {
		return controllerruntime.Result{}, err
	}
	klog.Infof("Finish handling WorkloadRebalancer (%s), %d/%d resource success in all, while %d resource need retry",
		rebalancer.Name, successNum, len(rebalancer.Status.ObservedWorkloads), retryNum)

	if retryNum > 0 {
		return controllerruntime.Result{}, fmt.Errorf("%d resource reschedule triggered failed and need retry", retryNum)
	}
	return controllerruntime.Result{}, nil
}

func (c *RebalancerController) buildWorkloadRebalancerStatus(rebalancer *appsv1alpha1.WorkloadRebalancer) appsv1alpha1.WorkloadRebalancerStatus {
	observedWorkloads := make([]appsv1alpha1.ObservedWorkload, 0)
	for _, resource := range rebalancer.Spec.Workloads {
		observedWorkloads = append(observedWorkloads, appsv1alpha1.ObservedWorkload{
			Workload: resource,
		})
	}
	return appsv1alpha1.WorkloadRebalancerStatus{ObservedWorkloads: observedWorkloads, ObservedGeneration: rebalancer.Generation}
}

// When spec filed of WorkloadRebalancer updated, we shall refresh the workload list in status.observedWorkloads:
//  1. a new workload added to spec list, just add it into status list too and do the rebalance.
//  2. a workload deleted from previous spec list, keep it in status list if already success, and remove it if not.
//  3. a workload is modified, just regard it as deleted an old one and inserted a new one.
//  4. just list order is disrupted, no additional action.
func (c *RebalancerController) syncWorkloadsFromSpecToStatus(rebalancer *appsv1alpha1.WorkloadRebalancer) appsv1alpha1.WorkloadRebalancerStatus {
	observedWorkloads := make([]appsv1alpha1.ObservedWorkload, 0)

	specWorkloads := sets.New[appsv1alpha1.ObjectReference]()
	for _, workload := range rebalancer.Spec.Workloads {
		specWorkloads.Insert(workload)
	}

	for _, item := range rebalancer.Status.ObservedWorkloads {
		// if item still exist in `spec`, keep it in `status` and remove it from `specWorkloads` set.
		// if item no longer exist in `spec`, keep it in `status` if it already success, otherwise remove it from `status`.
		if specWorkloads.Has(item.Workload) {
			observedWorkloads = append(observedWorkloads, item)
			specWorkloads.Delete(item.Workload)
		} else if item.Result == appsv1alpha1.RebalanceSuccessful {
			observedWorkloads = append(observedWorkloads, item)
		}
	}

	// since item exist in both `spec` and `status` has been removed, the left means the newly added workload,
	// add them into `status`.
	for workload := range specWorkloads {
		observedWorkloads = append(observedWorkloads, appsv1alpha1.ObservedWorkload{Workload: workload})
	}

	return appsv1alpha1.WorkloadRebalancerStatus{ObservedWorkloads: observedWorkloads, ObservedGeneration: rebalancer.Generation}
}

func (c *RebalancerController) doWorkloadRebalance(ctx context.Context, rebalancer *appsv1alpha1.WorkloadRebalancer) (successNum int64, retryNum int64) {
	// get previous status and update basing on it
	if len(rebalancer.Status.ObservedWorkloads) == 0 {
		rebalancer.Status = c.buildWorkloadRebalancerStatus(rebalancer)
	} else {
		rebalancer.Status = c.syncWorkloadsFromSpecToStatus(rebalancer)
	}

	successNum, retryNum = int64(0), int64(0)
	for i, resource := range rebalancer.Status.ObservedWorkloads {
		if resource.Result == appsv1alpha1.RebalanceSuccessful {
			successNum++
			continue
		}
		if resource.Result == appsv1alpha1.RebalanceFailed && resource.Reason == appsv1alpha1.RebalanceObjectNotFound {
			continue
		}

		bindingName := names.GenerateBindingName(resource.Workload.Kind, resource.Workload.Name)
		// resource with empty namespace represents it is a cluster wide resource.
		if resource.Workload.Namespace != "" {
			binding := &workv1alpha2.ResourceBinding{}
			if err := c.Client.Get(ctx, client.ObjectKey{Namespace: resource.Workload.Namespace, Name: bindingName}, binding); err != nil {
				klog.Errorf("get binding for resource %+v failed: %+v", resource.Workload, err)
				c.recordAndCountRebalancerFailed(&rebalancer.Status.ObservedWorkloads[i], &retryNum, err)
				continue
			}
			// update spec.rescheduleTriggeredAt of referenced fetchTargetRefBindings to trigger a rescheduling
			if c.needTriggerReschedule(rebalancer.CreationTimestamp, binding.Spec.RescheduleTriggeredAt) {
				binding.Spec.RescheduleTriggeredAt = &rebalancer.CreationTimestamp

				if err := c.Client.Update(ctx, binding); err != nil {
					klog.Errorf("update binding for resource %+v failed: %+v", resource.Workload, err)
					c.recordAndCountRebalancerFailed(&rebalancer.Status.ObservedWorkloads[i], &retryNum, err)
					continue
				}
			}
			c.recordAndCountRebalancerSuccess(&rebalancer.Status.ObservedWorkloads[i], &successNum)
		} else {
			clusterbinding := &workv1alpha2.ClusterResourceBinding{}
			if err := c.Client.Get(ctx, client.ObjectKey{Name: bindingName}, clusterbinding); err != nil {
				klog.Errorf("get cluster binding for resource %+v failed: %+v", resource.Workload, err)
				c.recordAndCountRebalancerFailed(&rebalancer.Status.ObservedWorkloads[i], &retryNum, err)
				continue
			}
			// update spec.rescheduleTriggeredAt of referenced clusterbinding to trigger a rescheduling
			if c.needTriggerReschedule(rebalancer.CreationTimestamp, clusterbinding.Spec.RescheduleTriggeredAt) {
				clusterbinding.Spec.RescheduleTriggeredAt = &rebalancer.CreationTimestamp

				if err := c.Client.Update(ctx, clusterbinding); err != nil {
					klog.Errorf("update cluster binding for resource %+v failed: %+v", resource.Workload, err)
					c.recordAndCountRebalancerFailed(&rebalancer.Status.ObservedWorkloads[i], &retryNum, err)
					continue
				}
			}
			c.recordAndCountRebalancerSuccess(&rebalancer.Status.ObservedWorkloads[i], &successNum)
		}
	}
	return
}

func (c *RebalancerController) needTriggerReschedule(creationTimestamp metav1.Time, rescheduleTriggeredAt *metav1.Time) bool {
	return rescheduleTriggeredAt == nil || creationTimestamp.After(rescheduleTriggeredAt.Time)
}

func (c *RebalancerController) recordAndCountRebalancerSuccess(resource *appsv1alpha1.ObservedWorkload, successNum *int64) {
	resource.Result = appsv1alpha1.RebalanceSuccessful
	*successNum++
}

func (c *RebalancerController) recordAndCountRebalancerFailed(resource *appsv1alpha1.ObservedWorkload, retryNum *int64, err error) {
	reason := apierrors.ReasonForError(err)
	if reason == metav1.StatusReasonNotFound {
		resource.Result = appsv1alpha1.RebalanceFailed
		resource.Reason = appsv1alpha1.RebalanceObjectNotFound
	} else {
		*retryNum++
	}
}

func (c *RebalancerController) updateWorkloadRebalancerStatus(ctx context.Context, oldRebalancer, rebalancer *appsv1alpha1.WorkloadRebalancer) error {
	if reflect.DeepEqual(oldRebalancer.Status, rebalancer.Status) {
		return nil
	}

	rebalancerPatch := client.MergeFrom(oldRebalancer)
	lastUpdateTime := metav1.Now()
	rebalancer.Status.LastUpdateTime = &lastUpdateTime

	return retry.RetryOnConflict(retry.DefaultRetry, func() (err error) {
		klog.V(4).Infof("Start to patch WorkloadRebalancer(%s) status", rebalancer.Name)
		if err = c.Client.Status().Patch(ctx, rebalancer, rebalancerPatch); err != nil {
			klog.Errorf("Failed to patch WorkloadRebalancer (%s) status, err: %+v", rebalancer.Name, err)
			return err
		}
		return nil
	})
}
