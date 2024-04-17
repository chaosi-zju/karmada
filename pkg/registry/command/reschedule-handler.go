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

package command

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commandapis "github.com/karmada-io/karmada/pkg/apis/command"
	"github.com/karmada-io/karmada/pkg/apis/command/validation"
	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	"github.com/karmada-io/karmada/pkg/util/helper"
	"github.com/karmada-io/karmada/pkg/util/names"
)

// RescheduleHandler REST handler of Reschedule
type RescheduleHandler struct {
	client client.Client
}

var _ Handler = &RescheduleHandler{}
var _ rest.Creater = &RescheduleHandler{}

// New returns an empty object that can be used with Create and Update after request data has been put into it.
// This object must be a pointer type for use with Codec.DecodeInto([]byte, runtime.Object)
func (h RescheduleHandler) New() runtime.Object {
	return &commandapis.Reschedule{}
}

// Destroy cleans up its resources on shutdown.
// Destroy has to be implemented in thread-safe way and be prepared
// for being called more than once.
func (h RescheduleHandler) Destroy() {}

// NamespaceScoped returns true if the storage is namespaced
func (h RescheduleHandler) NamespaceScoped() bool {
	return false
}

// GetSingularName returns singular name of resources.
func (h RescheduleHandler) GetSingularName() string {
	return commandapis.ResourceSingularReschedule
}

// Create creates a new version of a resource.
func (h RescheduleHandler) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	task, ok := obj.(*commandapis.Reschedule)
	if !ok {
		return nil, fmt.Errorf("obj is not commandapis.Reschedule type: %+v", obj)
	}
	klog.Infof("Reschedule (%s) Create called", task.Name)

	if errs := validation.ValidateReschedule(task); len(errs) > 0 {
		return task, errors.NewInvalid(task.GetObjectKind().GroupVersionKind().GroupKind(), task.GetName(), errs)
	}

	if createValidation != nil {
		if err := createValidation(ctx, task); err != nil {
			return task, err
		}
	}

	errs := field.ErrorList{}
	bindingMap := make(map[string]*workv1alpha2.ResourceBinding)
	clusterBindingMap := make(map[string]*workv1alpha2.ClusterResourceBinding)

	for _, policy := range task.Spec.TargetRefPolicy {
		// policy with empty namespace represents it is a cluster propagation policy
		if policy.Namespace != "" {
			bindinglist, err := helper.ListPPDerivedRB(ctx, h.client, policy.Namespace, policy.Name)
			if err != nil {
				errs = append(errs, field.NotFound(field.NewPath("xx"), err.Error()))
				continue
			}
			h.addBindingList(bindingMap, bindinglist.Items...)
		} else {
			bindinglist, err := helper.ListCPPDerivedRB(ctx, h.client, policy.Name)
			if err != nil {
				errs = append(errs, field.NotFound(field.NewPath("xx"), err.Error()))
				continue
			}
			h.addBindingList(bindingMap, bindinglist.Items...)

			clusterbindinglist, err := helper.ListCPPDerivedCRB(ctx, h.client, policy.Name)
			if err != nil {
				errs = append(errs, field.NotFound(field.NewPath("xx"), err.Error()))
				continue
			}
			h.addClusterBindingList(clusterBindingMap, clusterbindinglist.Items...)
		}
	}

	for _, resource := range task.Spec.TargetRefResource {
		bindingName := names.GenerateBindingName(resource.Kind, resource.Name)
		// policy with empty namespace represents it is a cluster propagation policy
		if resource.Namespace != "" {
			binding := workv1alpha2.ResourceBinding{}
			if err := h.client.Get(ctx, client.ObjectKey{Namespace: resource.Namespace, Name: bindingName}, &binding); err != nil {
				errs = append(errs, field.NotFound(field.NewPath("xx"), err.Error()))
				continue
			}
			h.addBindingList(bindingMap, binding)
		} else {
			clusterbinding := workv1alpha2.ClusterResourceBinding{}
			if err := h.client.Get(ctx, client.ObjectKey{Name: bindingName}, &clusterbinding); err != nil {
				errs = append(errs, field.NotFound(field.NewPath("xx"), err.Error()))
				continue
			}
			h.addClusterBindingList(clusterBindingMap, clusterbinding)
		}
	}

	for _, binding := range bindingMap {
		// update spec.rescheduleTriggeredAt of referenced binding to trigger a rescheduling
		binding.Spec.RescheduleTriggeredAt = metav1.Now()
		if err := h.client.Update(ctx, binding); err != nil {
			errs = append(errs, field.NotFound(field.NewPath("xx"), err.Error()))
			continue
		}
	}

	for _, clusterbinding := range clusterBindingMap {
		// update spec.rescheduleTriggeredAt of referenced clusterbinding to trigger a rescheduling
		clusterbinding.Spec.RescheduleTriggeredAt = metav1.Now()
		if err := h.client.Update(ctx, clusterbinding); err != nil {
			errs = append(errs, field.NotFound(field.NewPath("xx"), err.Error()))
			continue
		}
	}

	if len(errs) > 0 {
		klog.Infof("partly success")
		return obj, errors.NewInvalid(task.GetObjectKind().GroupVersionKind().GroupKind(), task.GetName(), errs)
	}

	klog.Infof("Successfully handled Reschedule (%s), %d bindings triggered rescheduling", task.Name, len(bindingMap)+len(clusterBindingMap))
	return obj, nil
}

func (h RescheduleHandler) addBindingList(bindingMap map[string]*workv1alpha2.ResourceBinding, items ...workv1alpha2.ResourceBinding) {
	for i := range items {
		bindingMap[fmt.Sprintf("%s/%s", items[i].Namespace, items[i].Name)] = &items[i]
	}
}

func (h RescheduleHandler) addClusterBindingList(clusterbindingMap map[string]*workv1alpha2.ClusterResourceBinding, items ...workv1alpha2.ClusterResourceBinding) {
	for i := range items {
		clusterbindingMap[items[i].Name] = &items[i]
	}
}

// GetRescheduleHandler returns a RescheduleHandler instance
func GetRescheduleHandler(controlPlaneClient client.Client) Handler {
	return &RescheduleHandler{client: controlPlaneClient}
}
