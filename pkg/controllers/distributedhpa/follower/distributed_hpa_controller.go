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

package follower

import (
	"context"
	"errors"
	"fmt"
	"time"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corelisters "k8s.io/client-go/listers/core/v1"
	scaleclient "k8s.io/client-go/scale"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	autoscalingv1alpha1 "github.com/karmada-io/karmada/pkg/apis/autoscaling/v1alpha1"
)

const (
	// ControllerName is the controller name that will be used when reporting events.
	ControllerName = "distributed-hpa-controller"
)

var (
	// errSpec is used to determine if the error comes from the spec of HPA object in reconcileAutoscaler.
	// All such errors should have this error as a root error so that the upstream function can distinguish spec errors from internal errors.
	// e.g., fmt.Errorf("invalid spec%w", errSpec)
	errSpec error = errors.New("")
)

type DistributedHPAController struct {
	client.Client // used to operate Cluster resources.

	ClusterName     string
	ScaleNamespacer scaleclient.ScalesGetter
	Mapper          apimeta.RESTMapper

	ReplicaCalc *ReplicaCalculator

	// PodLister is able to list/get Pods from the shared cache from the informer passed in to
	// NewHorizontalController.
	PodLister corelisters.PodLister

	ResyncPeriod time.Duration
}

var predicateFunc = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return true
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return true
	},
	GenericFunc: func(e event.GenericEvent) bool {
		return true
	},
}

// SetupWithManager creates a controller and register to controller manager.
func (a *DistributedHPAController) SetupWithManager(mgr controllerruntime.Manager) error {
	return controllerruntime.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&autoscalingv1alpha1.FederatedHPA{}, builder.WithPredicates(predicateFunc)).
		Complete(a)
}

// Reconcile performs a full reconciliation for the object referred to by the Request.
// The Controller will requeue the Request to be processed again if an error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (a *DistributedHPAController) Reconcile(ctx context.Context, req controllerruntime.Request) (controllerruntime.Result, error) {
	klog.Infof("[DEBUG] Reconcile the FederatedHPA of %+v", req.NamespacedName)

	hpa := &autoscalingv1alpha1.FederatedHPA{}
	if err := a.Client.Get(ctx, req.NamespacedName, hpa); err != nil {
		if apierrors.IsNotFound(err) {
			return controllerruntime.Result{}, nil
		}
		return controllerruntime.Result{}, err
	}

	scale, err := a.getScale(ctx, hpa)
	if err != nil {
		return controllerruntime.Result{}, err
	}

	clusterStatus, err := a.queryClusterMetrics(ctx, hpa, scale)
	if err != nil {
		return controllerruntime.Result{}, err
	}

	if err := a.updateStatusIfNeeded(ctx, hpa, a.ClusterName, clusterStatus); err != nil {
		return controllerruntime.Result{}, err
	}

	return controllerruntime.Result{RequeueAfter: a.ResyncPeriod}, nil
}

func (a *DistributedHPAController) getScale(ctx context.Context, hpa *autoscalingv1alpha1.FederatedHPA) (*autoscalingv1.Scale, error) {
	targetGV, err := schema.ParseGroupVersion(hpa.Spec.ScaleTargetRef.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid API version in scale target reference: %v%w", err, errSpec)
	}

	targetGK := schema.GroupKind{
		Group: targetGV.Group,
		Kind:  hpa.Spec.ScaleTargetRef.Kind,
	}

	mappings, err := a.Mapper.RESTMappings(targetGK)
	if err != nil {
		return nil, fmt.Errorf("unable to determine resource for scale target reference: %v", err)
	}

	scale, _, err := a.scaleForResourceMappings(ctx, hpa.Namespace, hpa.Spec.ScaleTargetRef.Name, mappings)
	if err != nil {
		return nil, fmt.Errorf("failed to query scale subresource for %+v: %v", hpa.Spec.ScaleTargetRef, err)
	}

	return scale, nil
}

func (a *DistributedHPAController) scaleForResourceMappings(ctx context.Context, namespace, name string, mappings []*apimeta.RESTMapping) (*autoscalingv1.Scale, schema.GroupResource, error) {
	var firstErr error
	for i, mapping := range mappings {
		targetGR := mapping.Resource.GroupResource()
		scale, err := a.ScaleNamespacer.Scales(namespace).Get(ctx, targetGR, name, metav1.GetOptions{})
		if err == nil {
			return scale, targetGR, nil
		}

		// if this is the first error, remember it,
		// then go on and try other mappings until we find a good one
		if i == 0 {
			firstErr = err
		}
	}

	// make sure we handle an empty set of mappings
	if firstErr == nil {
		firstErr = fmt.Errorf("unrecognized resource")
	}

	return nil, schema.GroupResource{}, firstErr
}

// queryClusterMetrics computes the desired number of replicas for the metric specifications listed in the HPA,
// returning the maximum of the computed replica counts, a description of the associated metric, and the statuses of
// all metrics computed.
// It may return both valid metricDesiredReplicas and an error,
// when some metrics still work and HPA should perform scaling based on them.
// If HPA cannot do anything due to error, it returns -1 in metricDesiredReplicas as a failure signal.
func (a *DistributedHPAController) queryClusterMetrics(ctx context.Context, hpa *autoscalingv1alpha1.FederatedHPA, scale *autoscalingv1.Scale) (*autoscalingv1alpha1.FederatedHPAClusterStatus, error) {
	selector, err := a.validateAndParseSelector(hpa, scale.Status.Selector)
	if err != nil {
		return nil, err
	}

	clusterStatus := autoscalingv1alpha1.FederatedHPAClusterStatus{ClusterMetric: make([]*autoscalingv1alpha1.ClusterMetric, len(hpa.Spec.Metrics))}
	for i, metricSpec := range hpa.Spec.Metrics {
		clusterMetric, err := a.queryClusterMetric(ctx, metricSpec, hpa, selector)

		if err != nil {
			// TODO resolve error
			klog.Errorf("queryClusterMetric failed: %+v", err)
			continue
		}

		clusterStatus.ClusterMetric[i] = clusterMetric
	}

	return &clusterStatus, nil
}

// updateStatusIfNeeded calls updateStatus only if the status of the new HPA is not the same as the old status
func (a *DistributedHPAController) updateStatusIfNeeded(ctx context.Context, hpa *autoscalingv1alpha1.FederatedHPA, clusterName string, clusterStatus *autoscalingv1alpha1.FederatedHPAClusterStatus) error {
	// skip write if we wouldn't need to update
	if apiequality.Semantic.DeepEqual(hpa.Status.ClusterStatus[clusterName], clusterStatus) {
		return nil
	}

	modifiedHPA := hpa.DeepCopy()
	if modifiedHPA.Status.ClusterStatus == nil {
		modifiedHPA.Status.ClusterStatus = make(map[string]autoscalingv1alpha1.FederatedHPAClusterStatus)
	}
	modifiedHPA.Status.ClusterStatus[clusterName] = *clusterStatus

	return retry.RetryOnConflict(retry.DefaultRetry, func() (err error) {
		klog.V(4).Infof("Start to update FederatedHPA(%s/%s) status", modifiedHPA.Namespace, modifiedHPA.Name)
		if err = a.Client.Status().Update(ctx, modifiedHPA); err != nil {
			klog.Errorf("Failed to update FederatedHPA(%s/%s) status, err: %+v", modifiedHPA.Namespace, modifiedHPA.Name, err)
			return err
		}
		klog.V(4).Infof("Update FederatedHPA(%s/%s) successful", modifiedHPA.Namespace, modifiedHPA.Name)
		return nil
	})
}

// validateAndParseSelector verifies that:
// - selector is not empty;
// - selector format is valid;
// - all pods by current selector are controlled by only one HPA.
// Returns an error if the check has failed or the parsed selector if succeeded.
// In case of an error the ScalingActive is set to false with the corresponding reason.
func (a *DistributedHPAController) validateAndParseSelector(hpa *autoscalingv1alpha1.FederatedHPA, selector string) (labels.Selector, error) {
	if selector == "" {
		errMsg := "selector is required"
		return nil, fmt.Errorf(errMsg)
	}

	parsedSelector, err := labels.Parse(selector)
	if err != nil {
		errMsg := fmt.Sprintf("couldn't convert selector into a corresponding internal selector object: %v", err)
		return nil, fmt.Errorf(errMsg)
	}

	//hpaKey := selectors.Key{Name: hpa.Name, Namespace: hpa.Namespace}
	//a.hpaSelectorsMux.Lock()
	//if a.hpaSelectors.SelectorExists(hpaKey) {
	//	// Update HPA selector only if the HPA was registered in enqueueHPA.
	//	a.hpaSelectors.PutSelector(hpaKey, parsedSelector)
	//}
	//a.hpaSelectorsMux.Unlock()

	//pods, err := a.PodLister.Pods(hpa.Namespace).List(parsedSelector)
	//if err != nil {
	//	return nil, err
	//}

	//selectingHpas := a.hpasControllingPodsUnderSelector(pods)
	//if len(selectingHpas) > 1 {
	//	errMsg := fmt.Sprintf("pods by selector %v are controlled by multiple HPAs: %v", selector, selectingHpas)
	//	return nil, fmt.Errorf(errMsg)
	//}

	return parsedSelector, nil
}

// Computes the desired number of replicas for a specific hpa and metric specification,
// returning the metric status and a proposed condition to be set on the HPA object.
func (a *DistributedHPAController) queryClusterMetric(ctx context.Context, spec autoscalingv2.MetricSpec, hpa *autoscalingv1alpha1.FederatedHPA,
	selector labels.Selector) (clusterMetric *autoscalingv1alpha1.ClusterMetric, err error) {

	switch spec.Type {
	case autoscalingv2.ResourceMetricSourceType:
		clusterMetric, err = a.computeStatusForResourceMetric(ctx, spec, hpa, selector)
		if err != nil {
			return clusterMetric, fmt.Errorf("failed to get %s resource metric value: %v", spec.Resource.Name, err)
		}
	default:
		// It shouldn't reach here as invalid metric source type is filtered out in the api-server's validation.
		err = fmt.Errorf("unknown metric source type %q%w", string(spec.Type), errSpec)
		return
	}

	return clusterMetric, nil
}

// computeStatusForResourceMetric computes the desired number of replicas for the specified metric of type ResourceMetricSourceType.
func (a *DistributedHPAController) computeStatusForResourceMetric(ctx context.Context, metricSpec autoscalingv2.MetricSpec, hpa *autoscalingv1alpha1.FederatedHPA,
	selector labels.Selector) (clusterMetric *autoscalingv1alpha1.ClusterMetric, err error) {
	clusterMetric, err = a.computeStatusForResourceMetricGeneric(ctx, metricSpec.Resource.Target, metricSpec.Resource.Name, hpa.Namespace, "", selector, autoscalingv2.ResourceMetricSourceType)
	if err != nil {
		return nil, err
	}

	return clusterMetric, nil
}

func (a *DistributedHPAController) computeStatusForResourceMetricGeneric(ctx context.Context, target autoscalingv2.MetricTarget,
	resourceName v1.ResourceName, namespace string, container string, selector labels.Selector, sourceType autoscalingv2.MetricSourceType) (
	clusterMetric *autoscalingv1alpha1.ClusterMetric, err error) {
	if target.AverageValue != nil {
		clusterMetric, err = a.ReplicaCalc.GetResourceMetric(ctx, resourceName, namespace, selector, container)
		if err != nil {
			return nil, fmt.Errorf("failed to get %s usage: %v", resourceName, err)
		}
		clusterMetric.MetricName = fmt.Sprintf("%s resource", resourceName.String())
		return clusterMetric, nil
	}

	if target.AverageUtilization == nil {
		errMsg := "invalid resource metric source: neither an average utilization target nor an average value (usage) target was set"
		return nil, fmt.Errorf(errMsg)
	}

	clusterMetric, err = a.ReplicaCalc.GetResourceMetric(ctx, resourceName, namespace, selector, container)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s utilization: %v", resourceName, err)
	}

	metricNameProposal := fmt.Sprintf("%s resource utilization (percentage of request)", resourceName)
	if sourceType == autoscalingv2.ContainerResourceMetricSourceType {
		metricNameProposal = fmt.Sprintf("%s container resource utilization (percentage of request)", resourceName)
	}
	clusterMetric.MetricName = metricNameProposal

	return clusterMetric, nil
}
