/*
Copyright 2021 The Karmada Authors.

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

package framework

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	appsv1alpha1 "github.com/karmada-io/karmada/pkg/apis/apps/v1alpha1"
	karmada "github.com/karmada-io/karmada/pkg/generated/clientset/versioned"
)

// CreateWorkloadRebalancer create WorkloadRebalancer with karmada client.
func CreateWorkloadRebalancer(client karmada.Interface, rebalancer *appsv1alpha1.WorkloadRebalancer) {
	ginkgo.By(fmt.Sprintf("Creating WorkloadRebalancer(%s)", rebalancer.Name), func() {
		newRebalancer, err := client.AppsV1alpha1().WorkloadRebalancers().Create(context.TODO(), rebalancer, metav1.CreateOptions{})
		*rebalancer = *newRebalancer
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	})
}

// RemoveWorkloadRebalancer delete WorkloadRebalancer.
func RemoveWorkloadRebalancer(client karmada.Interface, name string) {
	ginkgo.By(fmt.Sprintf("Removing WorkloadRebalancer(%s)", name), func() {
		err := client.AppsV1alpha1().WorkloadRebalancers().Delete(context.TODO(), name, metav1.DeleteOptions{})
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	})
}

// UpdateWorkloadRebalancer udpate WorkloadRebalancer with karmada client.
func UpdateWorkloadRebalancer(client karmada.Interface, name string, workloads []appsv1alpha1.ObjectReference) {
	ginkgo.By(fmt.Sprintf("Updating WorkloadRebalancer(%s)'s workloads", name), func() {
		gomega.Eventually(func() error {
			rebalancer, err := client.AppsV1alpha1().WorkloadRebalancers().Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			rebalancer.Spec.Workloads = workloads
			_, err = client.AppsV1alpha1().WorkloadRebalancers().Update(context.TODO(), rebalancer, metav1.UpdateOptions{})
			return err
		}, pollTimeout, pollInterval).ShouldNot(gomega.HaveOccurred())
	})
}

// WaitRebalancerObservedWorkloads wait observedWorkloads in WorkloadRebalancer fit with util timeout
func WaitRebalancerObservedWorkloads(client karmada.Interface, name string, expectedWorkloads []appsv1alpha1.ObservedWorkload) {
	ginkgo.By(fmt.Sprintf("Waiting for WorkloadRebalancer(%s) observedWorkload match to expected result", name), func() {
		gomega.Eventually(func() bool {
			rebalancer, err := client.AppsV1alpha1().WorkloadRebalancers().Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return false
			}
			if len(rebalancer.Status.ObservedWorkloads) == 0 {
				return false
			}
			return checkRebalancerObservedWorkloads(rebalancer.Status.ObservedWorkloads, expectedWorkloads)
		}, pollTimeout, pollInterval).Should(gomega.Equal(true))
	})
}

func checkRebalancerObservedWorkloads(observedWorkloads, expectedWorkloads []appsv1alpha1.ObservedWorkload) bool {
	gomega.Expect(len(observedWorkloads)).Should(gomega.Equal(len(expectedWorkloads)))

	expectedWorkloadsMap := make(map[string]appsv1alpha1.ObservedWorkload)
	for _, workload := range expectedWorkloads {
		expectedWorkloadsMap[genObjectReferenceKey(workload.Workload)] = workload
	}
	for _, workload := range observedWorkloads {
		expectedWorkload, exist := expectedWorkloadsMap[genObjectReferenceKey(workload.Workload)]
		if !exist {
			klog.Errorf("observedWorkloads: %+v", observedWorkloads)
			return false
		}
		if workload.Result != expectedWorkload.Result || workload.Reason != expectedWorkload.Reason {
			klog.Errorf("observedWorkloads: %+v", observedWorkloads)
			return false
		}
	}
	return true
}

func genObjectReferenceKey(obj appsv1alpha1.ObjectReference) string {
	if obj.Namespace == "" {
		return fmt.Sprintf("%s#%s#%s", obj.APIVersion, obj.Kind, obj.Name)
	}else{
		return fmt.Sprintf("%s#%s#%s#%s", obj.APIVersion, obj.Kind, obj.Name, obj.Namespace)
	}
}
