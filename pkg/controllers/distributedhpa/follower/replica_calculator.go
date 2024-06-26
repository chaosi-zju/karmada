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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	corelisters "k8s.io/client-go/listers/core/v1"

	autoscalingv1alpha1 "github.com/karmada-io/karmada/pkg/apis/autoscaling/v1alpha1"
	metricsclient "github.com/karmada-io/karmada/pkg/controllers/federatedhpa/metrics"
	"github.com/karmada-io/karmada/pkg/util/helper"
)

// This file is basically lifted from https://github.com/kubernetes/kubernetes/blob/release-1.27/pkg/controller/podautoscaler/replica_calculator.go.
// The main difference is:
// 1. ReplicaCalculator no longer has PodLister built in. PodList is calculated in the outer controller.
// 2. ReplicaCalculator needs to import a calibration value to calibrate the calculation results
// when they are determined by the global number of ready Pods or metrics.

// ReplicaCalculator bundles all needed information to calculate the target amount of replicas
type ReplicaCalculator struct {
	metricsClient                 metricsclient.QueryClient
	podLister                     corelisters.PodLister
	cpuInitializationPeriod       time.Duration
	delayOfInitialReadinessStatus time.Duration
}

// NewReplicaCalculator creates a new ReplicaCalculator and passes all necessary information to the new instance
func NewReplicaCalculator(metricsClient metricsclient.QueryClient, podLister corelisters.PodLister, cpuInitializationPeriod, delayOfInitialReadinessStatus time.Duration) *ReplicaCalculator {
	return &ReplicaCalculator{
		metricsClient:                 metricsClient,
		podLister:                     podLister,
		cpuInitializationPeriod:       cpuInitializationPeriod,
		delayOfInitialReadinessStatus: delayOfInitialReadinessStatus,
	}
}

// GetResourceMetric calculates the desired replica count based on a target resource usage (as a raw milli-value)
// for pods matching the given selector in the given namespace, and the current replica count
func (c *ReplicaCalculator) GetResourceMetric(ctx context.Context, resource corev1.ResourceName, namespace string, selector labels.Selector, container string) (*autoscalingv1alpha1.ClusterMetric, error) {
	metrics, timestamp, err := c.metricsClient.GetResourceMetric(ctx, resource, namespace, selector, container)
	if err != nil {
		return nil, fmt.Errorf("unable to get metrics for resource %s: %v", resource, err)
	}

	podList, err := c.podLister.Pods(namespace).List(selector)
	if err != nil {
		return nil, fmt.Errorf("unable to get pods while calculating replica count: %v", err)
	}

	if len(podList) == 0 {
		return nil, fmt.Errorf("no pods returned by selector while calculating replica count")
	}

	_, unreadyPods, missingPods, ignoredPods := groupPods(podList, metrics, resource, c.cpuInitializationPeriod, c.delayOfInitialReadinessStatus)
	removeMetricsForPods(metrics, ignoredPods)
	removeMetricsForPods(metrics, unreadyPods)

	if len(metrics) == 0 {
		return nil, fmt.Errorf("did not receive metrics for targeted pods (pods might be unready)")
	}

	averageValue := GetMetricsAverageValue(metrics)
	requests, err := calculatePodRequests(podList, container, resource)
	if err != nil {
		return nil, err
	}
	averageRequest, missingPodAverageRequest := GetAverageRequest(metrics, requests)

	return &autoscalingv1alpha1.ClusterMetric{
		AverageValue:     averageValue,
		MetricValueCount: len(metrics),
		MetricTimestamp:  metav1.NewTime(timestamp),

		AverageRequest:           &averageRequest,
		MissingPodAverageRequest: &missingPodAverageRequest,
		MissingPodsCount:         len(missingPods),
		UnreadyPodsCount:         len(unreadyPods),
	}, nil
}

func groupPods(pods []*corev1.Pod, metrics metricsclient.PodMetricsInfo, resource corev1.ResourceName, cpuInitializationPeriod, delayOfInitialReadinessStatus time.Duration) (readyPodCount int, unreadyPods, missingPods, ignoredPods sets.String) {
	missingPods = sets.NewString()
	unreadyPods = sets.NewString()
	ignoredPods = sets.NewString()
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodFailed {
			ignoredPods.Insert(pod.Name)
			continue
		}
		// Pending pods are ignored.
		if pod.Status.Phase == corev1.PodPending {
			unreadyPods.Insert(pod.Name)
			continue
		}
		// Pods missing metrics.
		metric, found := metrics[pod.Name]
		if !found {
			missingPods.Insert(pod.Name)
			continue
		}
		// Unready pods are ignored.
		if resource == corev1.ResourceCPU {
			var unready bool
			_, condition := helper.GetPodCondition(&pod.Status, corev1.PodReady)
			if condition == nil || pod.Status.StartTime == nil {
				unready = true
			} else {
				// Pod still within possible initialisation period.
				if pod.Status.StartTime.Add(cpuInitializationPeriod).After(time.Now()) {
					// Ignore sample if pod is unready or one window of metric wasn't collected since last state transition.
					unready = condition.Status == corev1.ConditionFalse || metric.Timestamp.Before(condition.LastTransitionTime.Time.Add(metric.Window))
				} else {
					// Ignore metric if pod is unready and it has never been ready.
					unready = condition.Status == corev1.ConditionFalse && pod.Status.StartTime.Add(delayOfInitialReadinessStatus).After(condition.LastTransitionTime.Time)
				}
			}
			if unready {
				unreadyPods.Insert(pod.Name)
				continue
			}
		}
		readyPodCount++
	}
	return
}

func calculatePodRequests(pods []*corev1.Pod, container string, resource corev1.ResourceName) (map[string]int64, error) {
	requests := make(map[string]int64, len(pods))
	for _, pod := range pods {
		podSum := int64(0)
		for _, c := range pod.Spec.Containers {
			if container == "" || container == c.Name {
				if containerRequest, ok := c.Resources.Requests[resource]; ok {
					podSum += containerRequest.MilliValue()
				} else {
					return nil, fmt.Errorf("missing request for %s in container %s of Pod %s", resource, c.Name, pod.ObjectMeta.Name)
				}
			}
		}
		requests[pod.Name] = podSum
	}
	return requests, nil
}

func removeMetricsForPods(metrics metricsclient.PodMetricsInfo, pods sets.String) {
	for _, pod := range pods.UnsortedList() {
		delete(metrics, pod)
	}
}

func GetMetricsAverageValue(metrics metricsclient.PodMetricsInfo) int64 {
	metricsTotal := int64(0)
	for _, metric := range metrics {
		metricsTotal += metric.Value
	}
	return metricsTotal / int64(len(metrics))
}

func GetAverageRequest(metrics metricsclient.PodMetricsInfo, requests map[string]int64) (int64, int64) {
	requestsCount, missingPodRequests := int64(0), int64(0)
	requestsTotal, missingPodRequestsTotal := int64(0), int64(0)
	for podName, request := range requests {
		if _, hasMetrics := metrics[podName]; hasMetrics {
			requestsTotal += request
			requestsCount++
		} else {
			missingPodRequestsTotal += request
			missingPodRequests++
		}
	}
	return requestsTotal / requestsCount, missingPodRequestsTotal / missingPodRequests
}
