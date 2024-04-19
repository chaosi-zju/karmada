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

package scheduletrigger

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	"github.com/karmada-io/karmada/pkg/util/helper"
	"github.com/karmada-io/karmada/pkg/util/names"
)

const (
	// ControllerName is the controller name that will be used when reporting events.
	ControllerName = "schedule-triggerObject"
)

// ScheduleTrigger is to triggerObject a rescheduling to resources selected by ScheduleTrigger object.
type ScheduleTrigger struct {
	Client client.Client
}

var predicateFunc = predicate.Funcs{
	CreateFunc:  func(e event.CreateEvent) bool { return true },
	UpdateFunc:  func(e event.UpdateEvent) bool { return false },
	DeleteFunc:  func(event.DeleteEvent) bool { return false },
	GenericFunc: func(event.GenericEvent) bool { return false },
}

// SetupWithManager creates a controller and register to controller manager.
func (s *ScheduleTrigger) SetupWithManager(mgr controllerruntime.Manager) error {
	return controllerruntime.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&policyv1alpha1.ScheduleTrigger{}, builder.WithPredicates(predicateFunc)).
		Complete(s)
}

// Reconcile performs a full reconciliation for the object referred to by the Request.
// The Controller will requeue the Request to be processed again if an error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (s *ScheduleTrigger) Reconcile(ctx context.Context, req controllerruntime.Request) (controllerruntime.Result, error) {
	klog.V(4).Infof("Reconciling for ScheduleTrigger %s", req.Name)

	// 1. get latest ScheduleTrigger
	trigger := &policyv1alpha1.ScheduleTrigger{}
	if err := s.Client.Get(ctx, req.NamespacedName, trigger); err != nil {
		if apierrors.IsNotFound(err) {
			klog.Infof("no need to reconcile ScheduleTrigger for it not found")
			return controllerruntime.Result{}, nil
		}
		return controllerruntime.Result{}, err
	}

	// 2. start ScheduleTrigger
	if trigger.Status.Phase == policyv1alpha1.TriggerPending {
		trigger.Status.Phase = policyv1alpha1.TriggerRunning
		if err := s.Client.Update(ctx, trigger); err != nil {
			return controllerruntime.Result{}, err
		}

	} else if trigger.Status.Phase == policyv1alpha1.TriggerSuccess {
		klog.Infof("ScheduleTrigger %s already finished", trigger.Name)
		return controllerruntime.Result{}, nil
	}

	var triggerCtx *triggerObject
	if trigger.Status.Phase == policyv1alpha1.TriggerFailed {
		triggerCtx = s.fetchTargetRefBindings(ctx, trigger.Status.FailedPolicyList, trigger.Status.FailedResourceList)
	} else {
		triggerCtx = s.fetchTargetRefBindings(ctx, trigger.Spec.TargetRefPolicy, trigger.Spec.TargetRefResource)
	}

	for _, binding := range triggerCtx.bindingMap {
		// update spec.rescheduleTriggeredAt of referenced fetchTargetRefBindings to triggerObject a rescheduling
		if trigger.CreationTimestamp.After(binding.Spec.RescheduleTriggeredAt.Time) {
			binding.Spec.RescheduleTriggeredAt = trigger.CreationTimestamp
			if err := s.Client.Update(ctx, binding); err != nil {
				triggerCtx.addfailedResource(binding.Spec.Resource)
				continue
			}
		}
	}
	for _, clusterbinding := range triggerCtx.clusterBindingMap {
		// update spec.rescheduleTriggeredAt of referenced clusterbinding to triggerObject a rescheduling
		if trigger.CreationTimestamp.After(clusterbinding.Spec.RescheduleTriggeredAt.Time) {
			clusterbinding.Spec.RescheduleTriggeredAt = trigger.CreationTimestamp
			if err := s.Client.Update(ctx, clusterbinding); err != nil {
				triggerCtx.addfailedResource(clusterbinding.Spec.Resource)
				continue
			}
		}
	}

	if err := s.updateTriggerResult(trigger, triggerCtx); err != nil {
		klog.Errorf("Failed to handle ScheduleTrigger (%s), %d failed policies, %d failed resources",
			trigger.Name, len(triggerCtx.failedPolicies), len(triggerCtx.failedResources))
		return controllerruntime.Result{}, err
	}

	klog.Infof("Successfully handled ScheduleTrigger (%s), %d bindings triggered rescheduling",
		trigger.Name, len(triggerCtx.bindingMap)+len(triggerCtx.clusterBindingMap))
	return controllerruntime.Result{}, nil
}

func (s *ScheduleTrigger) updateTriggerResult(trigger *policyv1alpha1.ScheduleTrigger, ctx *triggerObject) error {
	triggerCopy := trigger.DeepCopy()
	triggerPatch := client.MergeFrom(triggerCopy)

	if len(ctx.failedPolicies) > 0 || len(ctx.failedResources) > 0 {
		triggerCopy.Status.Phase = policyv1alpha1.TriggerFailed
	} else {
		triggerCopy.Status.Phase = policyv1alpha1.TriggerSuccess
	}
	triggerCopy.Status.FailedPolicyList = ctx.failedPolicies
	triggerCopy.Status.FailedResourceList = ctx.failedResources

	err := retry.RetryOnConflict(retry.DefaultRetry, func() (err error) {
		klog.V(4).Infof("Start to patch ScheduleTrigger(%s) status", trigger.Name)
		if err := s.Client.Patch(context.TODO(), triggerCopy, triggerPatch); err != nil {
			klog.Errorf("Failed to patch ScheduleTrigger (%s) status, err: %+v", trigger.Name, err)
			return err
		}
		return nil
	})

	return err
}

func (s *ScheduleTrigger) fetchTargetRefBindings(ctx context.Context, policies []policyv1alpha1.TargetRefPolicy,
	resources []policyv1alpha1.TargetRefResource) *triggerObject {
	obj := newTriggerObject()

	for _, policy := range policies {
		// policy with empty namespace represents it is a cluster propagation policy
		if policy.Namespace != "" {
			bindinglist, err := helper.ListPPDerivedRB(ctx, s.Client, policy.Namespace, policy.Name)
			if err != nil {
				obj.failedPolicies = append(obj.failedPolicies, policy)
				klog.Errorf("%+v", err)
				continue
			}
			obj.addBindingList(bindinglist.Items...)
		} else {
			bindinglist, err := helper.ListCPPDerivedRB(ctx, s.Client, policy.Name)
			if err != nil {
				obj.failedPolicies = append(obj.failedPolicies, policy)
				klog.Errorf("%+v", err)
				continue
			}
			obj.addBindingList(bindinglist.Items...)

			clusterbindinglist, err := helper.ListCPPDerivedCRB(ctx, s.Client, policy.Name)
			if err != nil {
				obj.failedPolicies = append(obj.failedPolicies, policy)
				klog.Errorf("%+v", err)
				continue
			}
			obj.addClusterBindingList(clusterbindinglist.Items...)
		}
	}

	for _, resource := range resources {
		bindingName := names.GenerateBindingName(resource.Kind, resource.Name)
		// policy with empty namespace represents it is a cluster propagation policy
		if resource.Namespace != "" {
			binding := workv1alpha2.ResourceBinding{}
			if err := s.Client.Get(ctx, client.ObjectKey{Namespace: resource.Namespace, Name: bindingName}, &binding); err != nil {
				obj.failedResources = append(obj.failedResources, resource)
				klog.Errorf("%+v", err)
				continue
			}
			obj.addBindingList(binding)
		} else {
			clusterbinding := workv1alpha2.ClusterResourceBinding{}
			if err := s.Client.Get(ctx, client.ObjectKey{Name: bindingName}, &clusterbinding); err != nil {
				obj.failedResources = append(obj.failedResources, resource)
				klog.Errorf("%+v", err)
				continue
			}
			obj.addClusterBindingList(clusterbinding)
		}
	}

	return obj
}

type triggerObject struct {
	bindingMap        map[string]*workv1alpha2.ResourceBinding
	clusterBindingMap map[string]*workv1alpha2.ClusterResourceBinding
	failedPolicies    []policyv1alpha1.TargetRefPolicy
	failedResources   []policyv1alpha1.TargetRefResource
}

func newTriggerObject() *triggerObject {
	return &triggerObject{
		bindingMap:        make(map[string]*workv1alpha2.ResourceBinding),
		clusterBindingMap: make(map[string]*workv1alpha2.ClusterResourceBinding),
		failedPolicies:    make([]policyv1alpha1.TargetRefPolicy, 0),
		failedResources:   make([]policyv1alpha1.TargetRefResource, 0),
	}
}

func (t *triggerObject) addBindingList(items ...workv1alpha2.ResourceBinding) {
	for i := range items {
		t.bindingMap[fmt.Sprintf("%s/%s", items[i].Namespace, items[i].Name)] = &items[i]
	}
}

func (t *triggerObject) addClusterBindingList(items ...workv1alpha2.ClusterResourceBinding) {
	for i := range items {
		t.clusterBindingMap[items[i].Name] = &items[i]
	}
}

func (t *triggerObject) addfailedResource(failedResource workv1alpha2.ObjectReference) {
	t.failedResources = append(t.failedResources, policyv1alpha1.TargetRefResource{
		APIVersion: failedResource.APIVersion,
		Kind:       failedResource.Kind,
		Name:       failedResource.Name,
		Namespace:  failedResource.Namespace,
	})
}
