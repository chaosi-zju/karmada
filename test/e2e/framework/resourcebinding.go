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

package framework

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	karmada "github.com/karmada-io/karmada/pkg/generated/clientset/versioned"
)

// WaitResourceBindingFitWith wait resourceBinding fit with util timeout
func WaitResourceBindingFitWith(client karmada.Interface, namespace, name string, fit func(resourceBinding *workv1alpha2.ResourceBinding) bool) {
	gomega.Eventually(func() bool {
		resourceBinding, err := client.WorkV1alpha2().ResourceBindings(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		return fit(resourceBinding)
	}, pollTimeout, pollInterval).Should(gomega.Equal(true))
}

// UpdateResourceBindingTargetClusters update resourceBinding's target clusters
func UpdateResourceBindingTargetClusters(client karmada.Interface, namespace, name string, clusters []workv1alpha2.TargetCluster) {
	ginkgo.By(fmt.Sprintf("Updating ResourceBinding(%s/%s)'s target clusters to %+v", namespace, name, clusters), func() {
		gomega.Eventually(func() error {
			binding, err := client.WorkV1alpha2().ResourceBindings(namespace).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			binding.Spec.Clusters = clusters
			_, err = client.WorkV1alpha2().ResourceBindings(namespace).Update(context.TODO(), binding, metav1.UpdateOptions{})
			return err
		}, pollTimeout, pollInterval).ShouldNot(gomega.HaveOccurred())
	})
}

// AssertBindingScheduleResult wait deployment present on member clusters sync with fit func.
func AssertBindingScheduleResult(client karmada.Interface, namespace, name string, expectedClusters []string, expectedResults [][]int32) {
	for _, expectedResult := range expectedResults {
		gomega.Expect(len(expectedResult)).Should(gomega.Equal(len(expectedClusters)))
	}
	ginkgo.By(fmt.Sprintf("Check ResourceBinding(%s/%s)'s target clusters is as expected", namespace, name), func() {
		gomega.Eventually(func() error {
			binding, err := client.WorkV1alpha2().ResourceBindings(namespace).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			scheduledResult := make([]int32, len(expectedClusters))
			for _, scheduledCluster := range binding.Spec.Clusters {
				scheduledClusterIsExpected := true
				for i, expectedCluster := range expectedClusters {
					if scheduledCluster.Name == expectedCluster {
						scheduledResult[i] = scheduledCluster.Replicas
						scheduledClusterIsExpected = true
						break
					}
				}
				if !scheduledClusterIsExpected {
					return fmt.Errorf("the scheduled cluster %+v is not in expected: %+v", scheduledCluster, expectedClusters)
				}
			}
			for _, expectedResult := range expectedResults {
				if replicaResultMatchExpected(scheduledResult, expectedResult) {
					return nil
				}
			}
			return fmt.Errorf("clusters %+v, scheduled result: %+v, expected possible results: %+v", expectedClusters, scheduledResult, expectedResults)
		}, pollTimeout, pollInterval).ShouldNot(gomega.HaveOccurred())
	})
}

func replicaResultMatchExpected(gotResult, expectedResult []int32) bool {
	gomega.Expect(len(gotResult)).Should(gomega.Equal(len(expectedResult)))
	for i := range gotResult {
		if gotResult[i] != expectedResult[i] {
			return false
		}
	}
	return true
}
