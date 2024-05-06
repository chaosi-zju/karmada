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

package e2e

import (
	"github.com/onsi/ginkgo/v2"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/utils/pointer"

	autoscalingv1alpha1 "github.com/karmada-io/karmada/pkg/apis/autoscaling/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	"github.com/karmada-io/karmada/test/e2e/framework"
	testhelper "github.com/karmada-io/karmada/test/helper"
)

var _ = ginkgo.Describe("testing for FederatedHPA and metrics-adapter", func() {
	var namespace string
	var deploymentName, serviceName, policyName, federatedHPAName, pressureToolPodName, targetCluster string
	var deployment *appsv1.Deployment
	var service *corev1.Service
	var policy *policyv1alpha1.ClusterPropagationPolicy
	var federatedHPA *autoscalingv1alpha1.FederatedHPA
	var pressureTool *corev1.Pod

	ginkgo.BeforeEach(func() {
		namespace = testNamespace
		deploymentName = deploymentNamePrefix + rand.String(RandomStrLength)
		serviceName = deploymentName
		policyName = deploymentName
		federatedHPAName = deploymentName
		pressureToolPodName = deploymentName
		targetCluster = framework.ClusterNames()[0]

		deployment = testhelper.NewDeployment(namespace, deploymentName)
		deployment.Spec.Replicas = pointer.Int32(1)
		deployment.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("20m"),
				corev1.ResourceMemory: resource.MustParse("20Mi"),
			}}
		service = testhelper.NewService(namespace, serviceName, corev1.ServiceTypeNodePort)
		pressureTool = testhelper.NewPod(namespace, pressureToolPodName)
		pressureTool.Spec.Containers = []corev1.Container{
			{
				Name:    pressureToolPodName,
				Image:   "alpine",
				Command: []string{"/bin/sh"},
				Args:    []string{"-c", "apk add curl; while true; do for i in `seq 200`; do curl http://" + serviceName + "." + namespace + ":80; done; sleep 1; done"},
			},
		}
		policy = testhelper.NewClusterPropagationPolicy(policyName, []policyv1alpha1.ResourceSelector{
			{
				APIVersion: deployment.APIVersion,
				Kind:       deployment.Kind,
				Name:       deploymentName,
			},
			{
				APIVersion: service.APIVersion,
				Kind:       service.Kind,
				Name:       serviceName,
			}, {
				APIVersion: pressureTool.APIVersion,
				Kind:       pressureTool.Kind,
				Name:       pressureToolPodName,
			}}, policyv1alpha1.Placement{
			ClusterAffinity: &policyv1alpha1.ClusterAffinity{
				ClusterNames: []string{targetCluster},
			},
		})
		federatedHPA = testhelper.NewFederatedHPA(namespace, federatedHPAName, deploymentName)
		federatedHPA.Spec.MaxReplicas = 10
		federatedHPA.Spec.Behavior.ScaleUp = &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: pointer.Int32(3)}
		federatedHPA.Spec.Behavior.ScaleDown = &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: pointer.Int32(3)}
		federatedHPA.Spec.Metrics[0].Resource.Target.AverageUtilization = pointer.Int32(10)
	})

	ginkgo.JustBeforeEach(func() {
		framework.CreateClusterPropagationPolicy(karmadaClient, policy)
		framework.CreateDeployment(kubeClient, deployment)
		framework.CreateService(kubeClient, service)
		framework.CreateFederatedHPA(karmadaClient, federatedHPA)

		ginkgo.DeferCleanup(func() {
			framework.RemoveFederatedHPA(karmadaClient, namespace, federatedHPAName)
			framework.RemoveService(kubeClient, namespace, serviceName)
			framework.RemoveDeployment(kubeClient, namespace, deploymentName)
			framework.RemoveClusterPropagationPolicy(karmadaClient, policyName)
			framework.WaitDeploymentDisappearOnCluster(targetCluster, namespace, deploymentName)
			framework.WaitServiceDisappearOnCluster(targetCluster, namespace, serviceName)
		})
	})

	ginkgo.Context("FederatedHPA scale Deployment", func() {
		ginkgo.It("do scale when deployment metrics of cpu/mem utilization up", func() {
			ginkgo.By("step1: check initial replicas result should equal to 1", func() {
				framework.WaitDeploymentStatus(kubeClient, deployment, 1)
			})

			ginkgo.By("step2: pressure test the deployment of member clusters to increase the cpu/mem", func() {
				framework.CreatePod(kubeClient, pressureTool)
				framework.WaitPodPresentOnClusterFitWith(targetCluster, namespace, pressureToolPodName, func(_ *corev1.Pod) bool { return true })
				ginkgo.DeferCleanup(func() {
					framework.RemovePod(kubeClient, namespace, pressureToolPodName)
					framework.WaitPodDisappearOnCluster(targetCluster, namespace, pressureToolPodName)
				})
			})

			ginkgo.By("step3: check final replicas result should greater than 1", func() {
				framework.WaitDeploymentFitWith(kubeClient, namespace, deploymentName, func(deploy *appsv1.Deployment) bool {
					return *deploy.Spec.Replicas > 1 && deploy.Status.Replicas > 1
				})
			})
		})
	})
})
