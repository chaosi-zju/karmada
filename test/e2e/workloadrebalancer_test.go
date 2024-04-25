/*
Copyright 2023 The Karmada Authors.

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

package e2e

import (
	"sort"

	"github.com/onsi/ginkgo/v2"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/klog/v2"

	appsv1alpha1 "github.com/karmada-io/karmada/pkg/apis/apps/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	"github.com/karmada-io/karmada/pkg/util"
	"github.com/karmada-io/karmada/pkg/util/names"
	"github.com/karmada-io/karmada/test/e2e/framework"
	"github.com/karmada-io/karmada/test/helper"
)

// test case dimension:
//
//	schedule strategy: static weight, dyncmic weight, aggregated
//	resource type: workload type like deployment, non-workload type like clusterrole
//	expected result: successful, not found failure
var _ = ginkgo.Describe("workload rebalancer testing", func() {
	var namespace, unknownNamespace string
	var deploy1Name, deploy1BindingName, deploy2Name string
	var clusterroleName, clusterroleBindingName string
	var cppName, rebalancerName string
	var deploy1Obj, deploy2Obj, notExistDeployObj, clusterroleObj appsv1alpha1.ObjectReference

	var deploy1, deploy2 *appsv1.Deployment
	var clusterrole *v1.ClusterRole
	var policy *policyv1alpha1.ClusterPropagationPolicy
	var rebalancer *appsv1alpha1.WorkloadRebalancer
	var targetClusters []string

	ginkgo.BeforeEach(func() {
		namespace = testNamespace
		unknownNamespace = testNamespace + rand.String(RandomStrLength)
		deploy1Name = deploymentNamePrefix + rand.String(RandomStrLength)
		deploy2Name = deploymentNamePrefix + rand.String(RandomStrLength)
		clusterroleName = clusterRoleNamePrefix + rand.String(RandomStrLength)
		deploy1BindingName = names.GenerateBindingName(util.DeploymentKind, deploy1Name)
		clusterroleBindingName = names.GenerateBindingName(util.ClusterRoleKind, clusterroleName)
		cppName = deploy1Name
		rebalancerName = deploy1Name

		// sort member clusters in increasing order
		targetClusters = framework.ClusterNames()[0:2]
		sort.Strings(targetClusters)

		deploy1 = helper.NewDeployment(namespace, deploy1Name)
		deploy2 = helper.NewDeployment(namespace, deploy2Name)
		clusterrole = helper.NewClusterRole(clusterroleName, nil)
		policy = helper.NewClusterPropagationPolicy(cppName, []policyv1alpha1.ResourceSelector{
			{APIVersion: deploy1.APIVersion, Kind: deploy1.Kind, Name: deploy1.Name, Namespace: deploy1.Namespace},
			{APIVersion: deploy2.APIVersion, Kind: deploy2.Kind, Name: deploy2.Name, Namespace: deploy2.Namespace},
			{APIVersion: clusterrole.APIVersion, Kind: clusterrole.Kind, Name: clusterrole.Name},
		}, policyv1alpha1.Placement{
			ClusterAffinity: &policyv1alpha1.ClusterAffinity{ClusterNames: targetClusters},
		})

		deploy1Obj = appsv1alpha1.ObjectReference{APIVersion: deploy1.APIVersion, Kind: deploy1.Kind, Name: deploy1.Name, Namespace: deploy1.Namespace}
		deploy2Obj = appsv1alpha1.ObjectReference{APIVersion: deploy2.APIVersion, Kind: deploy2.Kind, Name: deploy2.Name, Namespace: deploy2.Namespace}
		notExistDeployObj = appsv1alpha1.ObjectReference{APIVersion: deploy1.APIVersion, Kind: deploy1.Kind, Name: deploy1.Name, Namespace: unknownNamespace}
		clusterroleObj = appsv1alpha1.ObjectReference{APIVersion: clusterrole.APIVersion, Kind: clusterrole.Kind, Name: clusterrole.Name}

		rebalancer = helper.NewWorkloadRebalancer(rebalancerName, []appsv1alpha1.ObjectReference{deploy1Obj, clusterroleObj, notExistDeployObj})
	})

	ginkgo.JustBeforeEach(func() {
		framework.CreateClusterPropagationPolicy(karmadaClient, policy)
		framework.CreateDeployment(kubeClient, deploy1)
		framework.CreateDeployment(kubeClient, deploy2)
		framework.CreateClusterRole(kubeClient, clusterrole)

		ginkgo.DeferCleanup(func() {
			framework.RemoveClusterPropagationPolicy(karmadaClient, policy.Name)
			framework.RemoveDeployment(kubeClient, deploy1.Namespace, deploy1.Name)
			framework.RemoveDeployment(kubeClient, deploy2.Namespace, deploy2.Name)
			framework.RemoveClusterRole(kubeClient, clusterrole.Name)
			framework.WaitDeploymentDisappearOnClusters(targetClusters, deploy1.Namespace, deploy1.Name)
			framework.WaitDeploymentDisappearOnClusters(targetClusters, deploy2.Namespace, deploy2.Name)
		})
	})

	var checkRescheduleResult = func(deploy1PossibleReplicas [][]int32, expectedWorkloads []appsv1alpha1.ObservedWorkload) {
		// 1. check rebalancer status: match to `expectedWorkloads`.
		framework.WaitRebalancerObservedWorkloads(karmadaClient, rebalancerName, expectedWorkloads)
		// 2. check deploy1: referenced binding's `spec.rescheduleTriggeredAt` and `status.lastScheduledTime` should be
		// updated + actual distribution of replicas should change.
		framework.WaitResourceBindingFitWith(karmadaClient, namespace, deploy1BindingName, func(rb *workv1alpha2.ResourceBinding) bool {
			return bindingHasRescheduled(rb.Spec, rb.Status, rebalancer.CreationTimestamp)
		})
		framework.AssertBindingScheduleResult(karmadaClient, namespace, deploy1BindingName, targetClusters, deploy1PossibleReplicas)
		// 3. check clusterrole: referenced binding's `spec.rescheduleTriggeredAt` and `status.lastScheduledTime` should be updated.
		framework.WaitClusterResourceBindingFitWith(karmadaClient, clusterroleBindingName, func(crb *workv1alpha2.ClusterResourceBinding) bool {
			return bindingHasRescheduled(crb.Spec, crb.Status, rebalancer.CreationTimestamp)
		})
	}

	// 1. static weight scheduling
	ginkgo.Context("static weight schedule type", func() {
		ginkgo.BeforeEach(func() {
			policy.Spec.Placement.ReplicaScheduling = helper.NewStaticWeightPolicyStrategy(targetClusters, []int64{2, 1})
		})

		ginkgo.It("reschedule when policy is static weight schedule type", func() {
			ginkgo.By("step1: check first schedule result", func() {
				// after first schedule, deployment is assigned as 2:1 in target clusters and clusterrole propagated to each cluster.
				framework.AssertBindingScheduleResult(karmadaClient, namespace, deploy1BindingName, targetClusters, [][]int32{{2, 1}})
				framework.WaitClusterRolePresentOnClustersFitWith(targetClusters, clusterroleName, func(_ *v1.ClusterRole) bool { return true })
			})

			ginkgo.By("step2: manually modify the schedule result", func() {
				// directly modify deployment schedule result to mock failover when cluster failure and recovery.
				// clusterrole not support this way, so skip mock clusterrole failover.
				modifiedClusters := []workv1alpha2.TargetCluster{{Name: targetClusters[0], Replicas: *deploy1.Spec.Replicas}}
				framework.UpdateResourceBindingTargetClusters(karmadaClient, namespace, deploy1BindingName, modifiedClusters)

				framework.AssertBindingScheduleResult(karmadaClient, namespace, deploy1BindingName, targetClusters, [][]int32{{3, 0}})
			})

			ginkgo.By("step3: trigger a reschedule by WorkloadRebalancer", func() {
				framework.CreateWorkloadRebalancer(karmadaClient, rebalancer)
				ginkgo.DeferCleanup(func() {
					framework.RemoveWorkloadRebalancer(karmadaClient, rebalancerName)
				})

				expectedWorkloads := []appsv1alpha1.ObservedWorkload{
					{Workload: deploy1Obj, Result: appsv1alpha1.RebalanceSuccessful},
					{Workload: clusterroleObj, Result: appsv1alpha1.RebalanceSuccessful},
					{Workload: notExistDeployObj, Result: appsv1alpha1.RebalanceFailed, Reason: appsv1alpha1.RebalanceObjectNotFound},
				}
				checkRescheduleResult([][]int32{{2, 1}}, expectedWorkloads)
			})

			ginkgo.By("step4: udpate WorkloadRebalancer spec workloads", func() {
				// update workload list from {deploy1, clusterrole, notExistDeployObj} to {clusterroleObj, deploy2Obj}
				updatedWorkloads := []appsv1alpha1.ObjectReference{clusterroleObj, deploy2Obj}
				framework.UpdateWorkloadRebalancer(karmadaClient, rebalancerName, updatedWorkloads)

				// according to current implementation, update action has no response, so keep original status.
				// TODO to be adjusted when update implemented
				expectedWorkloads := []appsv1alpha1.ObservedWorkload{
					{Workload: deploy1Obj, Result: appsv1alpha1.RebalanceSuccessful},
					{Workload: clusterroleObj, Result: appsv1alpha1.RebalanceSuccessful},
					{Workload: notExistDeployObj, Result: appsv1alpha1.RebalanceFailed, Reason: appsv1alpha1.RebalanceObjectNotFound},
				}
				framework.WaitRebalancerObservedWorkloads(karmadaClient, rebalancerName, expectedWorkloads)
			})
		})
	})

	// 2. dynamic weight scheduling
	ginkgo.Context("dynamic weight schedule type", func() {
		ginkgo.BeforeEach(func() {
			policy.Spec.Placement.ReplicaScheduling = &policyv1alpha1.ReplicaSchedulingStrategy{
				ReplicaSchedulingType:     policyv1alpha1.ReplicaSchedulingTypeDivided,
				ReplicaDivisionPreference: policyv1alpha1.ReplicaDivisionPreferenceWeighted,
				WeightPreference: &policyv1alpha1.ClusterPreferences{
					DynamicWeight: policyv1alpha1.DynamicWeightByAvailableReplicas,
				},
			}
		})

		ginkgo.It("reschedule when policy is dynamic weight schedule type", func() {
			ginkgo.By("step1: check first schedule result", func() {
				// after first schedule, deployment is assigned as 1:2 or 2:1 in target clusters and clusterrole propagated to each cluster.
				framework.AssertBindingScheduleResult(karmadaClient, namespace, deploy1BindingName, targetClusters, [][]int32{{1, 2}, {2, 1}})
				framework.WaitClusterRolePresentOnClustersFitWith(targetClusters, clusterroleName, func(_ *v1.ClusterRole) bool { return true })
			})

			ginkgo.By("step2: manually modify the schedule result", func() {
				// directly modify deployment schedule result to mock failover when cluster failure and recovery.
				// clusterrole not support this way, so skip mock clusterrole failover.
				modifiedClusters := []workv1alpha2.TargetCluster{{Name: targetClusters[0], Replicas: *deploy1.Spec.Replicas}}
				framework.UpdateResourceBindingTargetClusters(karmadaClient, namespace, deploy1BindingName, modifiedClusters)

				framework.AssertBindingScheduleResult(karmadaClient, namespace, deploy1BindingName, targetClusters, [][]int32{{3, 0}})
			})

			ginkgo.By("step3: trigger a reschedule by WorkloadRebalancer", func() {
				framework.CreateWorkloadRebalancer(karmadaClient, rebalancer)
				ginkgo.DeferCleanup(func() {
					framework.RemoveWorkloadRebalancer(karmadaClient, rebalancerName)
				})

				expectedWorkloads := []appsv1alpha1.ObservedWorkload{
					{Workload: deploy1Obj, Result: appsv1alpha1.RebalanceSuccessful},
					{Workload: clusterroleObj, Result: appsv1alpha1.RebalanceSuccessful},
					{Workload: notExistDeployObj, Result: appsv1alpha1.RebalanceFailed, Reason: appsv1alpha1.RebalanceObjectNotFound},
				}
				checkRescheduleResult([][]int32{{1, 2}, {2, 1}}, expectedWorkloads)
			})

			ginkgo.By("step4: udpate WorkloadRebalancer spec workloads", func() {
				// update workload list from {deploy1, clusterrole, notExistDeployObj} to {clusterroleObj, deploy2Obj}
				updatedWorkloads := []appsv1alpha1.ObjectReference{clusterroleObj, deploy2Obj}
				framework.UpdateWorkloadRebalancer(karmadaClient, rebalancerName, updatedWorkloads)

				// according to current implementation, update action has no response, so keep original status.
				// TODO to be adjusted when update implemented
				expectedWorkloads := []appsv1alpha1.ObservedWorkload{
					{Workload: deploy1Obj, Result: appsv1alpha1.RebalanceSuccessful},
					{Workload: clusterroleObj, Result: appsv1alpha1.RebalanceSuccessful},
					{Workload: notExistDeployObj, Result: appsv1alpha1.RebalanceFailed, Reason: appsv1alpha1.RebalanceObjectNotFound},
				}
				framework.WaitRebalancerObservedWorkloads(karmadaClient, rebalancerName, expectedWorkloads)
			})
		})
	})

	// 3. aggregated scheduling
	ginkgo.Context("aggregated schedule type", func() {
		ginkgo.BeforeEach(func() {
			policy.Spec.Placement.ReplicaScheduling = &policyv1alpha1.ReplicaSchedulingStrategy{
				ReplicaSchedulingType:     policyv1alpha1.ReplicaSchedulingTypeDivided,
				ReplicaDivisionPreference: policyv1alpha1.ReplicaDivisionPreferenceAggregated,
			}
		})

		ginkgo.It("reschedule when policy is aggregated schedule type", func() {
			ginkgo.By("step1: check first schedule result", func() {
				// after first schedule, deployment is assigned as 3:0 or 0:3 in target clusters and clusterrole propagated to each cluster.
				framework.AssertBindingScheduleResult(karmadaClient, namespace, deploy1BindingName, targetClusters, [][]int32{{3, 0}, {0, 3}})
				framework.WaitClusterRolePresentOnClustersFitWith(targetClusters, clusterroleName, func(_ *v1.ClusterRole) bool { return true })
			})

			ginkgo.By("step2: manually modify the schedule result", func() {
				// directly modify deployment schedule result to mock failover when cluster failure and recovery.
				// clusterrole not support this way, so skip mock clusterrole failover.
				modifiedClusters := []workv1alpha2.TargetCluster{{Name: targetClusters[0], Replicas: 2}, {Name: targetClusters[1], Replicas: 1}}
				framework.UpdateResourceBindingTargetClusters(karmadaClient, namespace, deploy1BindingName, modifiedClusters)

				framework.AssertBindingScheduleResult(karmadaClient, namespace, deploy1BindingName, targetClusters, [][]int32{{2, 1}})
			})

			ginkgo.By("step3: trigger a reschedule by WorkloadRebalancer", func() {
				framework.CreateWorkloadRebalancer(karmadaClient, rebalancer)
				ginkgo.DeferCleanup(func() {
					framework.RemoveWorkloadRebalancer(karmadaClient, rebalancerName)
				})

				expectedWorkloads := []appsv1alpha1.ObservedWorkload{
					{Workload: deploy1Obj, Result: appsv1alpha1.RebalanceSuccessful},
					{Workload: clusterroleObj, Result: appsv1alpha1.RebalanceSuccessful},
					{Workload: notExistDeployObj, Result: appsv1alpha1.RebalanceFailed, Reason: appsv1alpha1.RebalanceObjectNotFound},
				}
				checkRescheduleResult([][]int32{{3, 0}, {0, 3}}, expectedWorkloads)
			})

			ginkgo.By("step4: udpate WorkloadRebalancer spec workloads", func() {
				// update workload list from {deploy1, clusterrole, notExistDeployObj} to {clusterroleObj, deploy2Obj}
				updatedWorkloads := []appsv1alpha1.ObjectReference{clusterroleObj, deploy2Obj}
				framework.UpdateWorkloadRebalancer(karmadaClient, rebalancerName, updatedWorkloads)

				// according to current implementation, update action has no response, so keep original status.
				// TODO to be adjusted when update implemented
				expectedWorkloads := []appsv1alpha1.ObservedWorkload{
					{Workload: deploy1Obj, Result: appsv1alpha1.RebalanceSuccessful},
					{Workload: clusterroleObj, Result: appsv1alpha1.RebalanceSuccessful},
					{Workload: notExistDeployObj, Result: appsv1alpha1.RebalanceFailed, Reason: appsv1alpha1.RebalanceObjectNotFound},
				}
				framework.WaitRebalancerObservedWorkloads(karmadaClient, rebalancerName, expectedWorkloads)
			})
		})
	})
})

func bindingHasRescheduled(spec workv1alpha2.ResourceBindingSpec, status workv1alpha2.ResourceBindingStatus, rebalancerCreationTime metav1.Time) bool {
	if *spec.RescheduleTriggeredAt != rebalancerCreationTime || status.LastScheduledTime.Before(spec.RescheduleTriggeredAt) {
		klog.Errorf("rebalancerCreationTime: %+v, rescheduleTriggeredAt / lastScheduledTime: %+v / %+v",
			rebalancerCreationTime, *spec.RescheduleTriggeredAt, status.LastScheduledTime)
		return false
	}
	return true
}
