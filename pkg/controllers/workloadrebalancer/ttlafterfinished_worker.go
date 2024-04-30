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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/karmada-io/karmada/pkg/apis/apps/v1alpha1"
	"github.com/karmada-io/karmada/pkg/util"
)

func (c *RebalancerController) deleteExpiredRebalancer(key util.QueueKey) error {
	objKey, ok := key.(client.ObjectKey)
	if !ok {
		klog.Errorf("Invalid object key: %+v", key)
		return nil
	}
	klog.V(4).Infof("Checking if WorkloadRebalancer(%s) is ready for cleanup", objKey.Name)

	// 1. Get WorkloadRebalancer from cache.
	rebalancer := &appsv1alpha1.WorkloadRebalancer{}
	if err := c.Client.Get(context.TODO(), objKey, rebalancer); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// 2. Use the WorkloadRebalancer from cache to see if the TTL expires.
	if expiredAt, err := c.processTTL(rebalancer); err != nil {
		return err
	} else if expiredAt == nil {
		return nil
	}

	// 3. The WorkloadRebalancer's TTL is assumed to have expired, but the WorkloadRebalancer TTL might be stale.
	// Before deleting the WorkloadRebalancer, do a final sanity check.
	// If TTL is modified before we do this check, we cannot be sure if the TTL truly expires.
	fresh := &appsv1alpha1.WorkloadRebalancer{}
	if err := c.ControlPlaneClient.Get(context.TODO(), objKey, fresh); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// 4. Use the latest WorkloadRebalancer directly from api server to see if the TTL truly expires.
	if expiredAt, err := c.processTTL(fresh); err != nil {
		return err
	} else if expiredAt == nil {
		return nil
	}

	// 5. deletes the WorkloadRebalancer if TTL truly expires.
	options := &client.DeleteOptions{Preconditions: &metav1.Preconditions{ResourceVersion: &fresh.ResourceVersion}}
	if err := c.ControlPlaneClient.Delete(context.TODO(), fresh, options); err != nil {
		klog.Errorf("Cleaning up WorkloadRebalancer(%s) failed: %+v", fresh.Name, err)
		return err
	}
	klog.V(4).Infof("Cleaning up WorkloadRebalancer(%s) successful.", fresh.Name)

	return nil
}

// processTTL checks whether a given WorkloadRebalancer's TTL has expired, and add it to the queue after
// the TTL is expected to expire if the TTL will expire later.
func (c *RebalancerController) processTTL(r *appsv1alpha1.WorkloadRebalancer) (expiredAt *time.Time, err error) {
	if !needsCleanup(r) {
		return nil, nil
	}

	remainingTTL, expireAt, err := timeLeft(r)
	if err != nil {
		return nil, err
	}

	// TTL has expired
	if *remainingTTL <= 0 {
		return expireAt, nil
	}

	c.ttlAfterFinishedWorker.AddAfter(client.ObjectKey{Name: r.Name}, *remainingTTL)
	return nil, nil
}

func timeLeft(r *appsv1alpha1.WorkloadRebalancer) (*time.Duration, *time.Time, error) {
	now := time.Now()
	finishAt := r.Status.LastUpdateTime.Time
	expireAt := finishAt.Add(time.Duration(*r.Spec.TTLMinutesAfterFinished) * time.Minute)

	if finishAt.After(now) {
		klog.Infof("Found Rebalancer(%s) finished in the future. This is likely due to time skew in the cluster, cleanup will be deferred.", r.Name)
	}

	remainingTTL := expireAt.Sub(now)
	klog.V(4).Infof("Found Rebalancer(%s) finished, finishTime: %+v, remainingTTL: %+v, startTime: %+v, deadlineTTL: %+v",
		r.Name, finishAt.UTC(), remainingTTL, now.UTC(), expireAt.UTC())

	return &remainingTTL, &expireAt, nil
}

// needsCleanup checks whether a WorkloadRebalancer has finished and has a TTL set.
func needsCleanup(r *appsv1alpha1.WorkloadRebalancer) bool {
	return r.Spec.TTLMinutesAfterFinished != nil && isRebalancerFinished(r)
}

// isRebalancerFinished checks whether the given WorkloadRebalancer has finished execution.
// It does not discriminate between successful and failed terminations.
func isRebalancerFinished(r *appsv1alpha1.WorkloadRebalancer) bool {
	// if a finished WorkloadRebalancer is updated and didn't have time to refresh the status,
	// it is regarded as non-finished since observedGeneration not equal to generation.
	if r.Status.ObservedGeneration != r.Generation {
		return false
	}
	for _, workload := range r.Status.ObservedWorkloads {
		if workload.Result != appsv1alpha1.RebalanceSuccessful && workload.Result != appsv1alpha1.RebalanceFailed {
			return false
		}
	}
	return true
}
