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
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	"github.com/prometheus/common/model"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	"github.com/karmada-io/karmada/pkg/util/names"
	"github.com/karmada-io/karmada/test/e2e/framework"
	testhelper "github.com/karmada-io/karmada/test/helper"
)

const (
	workDurationSumMetric          = "workqueue_work_duration_seconds_sum"
	workDurationCountMetric        = "workqueue_work_duration_seconds_count"
	resourceMatchPolicySumMetric   = "resource_match_policy_duration_seconds_sum"
	resourceMatchPolicyCountMetric = "resource_match_policy_duration_seconds_count"
	propagationPolicyReconciler    = "propagationPolicy reconciler"
	workqueueDepth                 = "workqueue_depth"
	//workqueueWorkDurationSecondsBucket = "workqueue_work_duration_seconds_bucket"
)

var _ = framework.SerialDescribe("detector benchmark testing", func() {
	ginkgo.Context("detector reconcile benchmark", func() {
		ginkgo.It("policy update testing", func() {
			grabber, err := framework.NewMetricsGrabber(context.TODO(), hostKubeClient)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			experiment := gmeasure.NewExperiment("policy update performance")
			ginkgo.AddReportEntry(experiment.Name, experiment)

			startSumMetric, startCountMetric := getDurationMetric(grabber, workDurationSumMetric, workDurationCountMetric, propagationPolicyReconciler)

			experiment.RecordNote("detector reconcile time")
			deployments := []*appsv1.Deployment{}
			policies := []*policyv1alpha1.PropagationPolicy{}
			experiment.Sample(func(i int) {
				deployment := createDeployment(testNamespace, fmt.Sprintf("%s%s-%d", deploymentNamePrefix, rand.String(RandomStrLength), i+1))
				policy := propagateDeployment(deployment)
				policy.Spec.ConflictResolution = policyv1alpha1.ConflictOverwrite
				framework.UpdatePropagationPolicyWithSpec(karmadaClient, policy.Namespace, policy.Name, policy.Spec)
				deployments = append(deployments, deployment)
				policies = append(policies, policy)
			}, gmeasure.SamplingConfig{N: 50, NumParallel: 10})

			for i, deployment := range deployments {
				framework.RemovePropagationPolicy(karmadaClient, policies[i].Namespace, policies[i].Name)
				framework.RemoveDeployment(kubeClient, deployment.Namespace, deployment.Name)
				framework.WaitDeploymentDisappearOnClusters(framework.ClusterNames(), deployment.Namespace, deployment.Name)
			}

			waitWorkQueueFinished(grabber, workqueueDepth, propagationPolicyReconciler)

			endSumMetric, endCountMetric := getDurationMetric(grabber, workDurationSumMetric, workDurationCountMetric, propagationPolicyReconciler)
			klog.Infof("startSumMetric: %+v, startCountMetric: %+v, endSumMetric: %+v, endCountMetric: %+v",
				startSumMetric.Value, startCountMetric.Value, endSumMetric.Value, endCountMetric.Value)

			secondsSum := endSumMetric.Value - startSumMetric.Value
			secondsCnt := endCountMetric.Value - startCountMetric.Value
			duration := (secondsSum / secondsCnt) * 1000
			klog.Infof("secondsSum: %+v, secondsCnt: %+v, duration: %+v", secondsSum, secondsCnt, duration)

			experiment.RecordValue("secondsSum(s)", float64(secondsSum), gmeasure.Style("{{red}}"), gmeasure.Precision(time.Second))
			experiment.RecordValue("secondsCnt", float64(secondsCnt), gmeasure.Style("{{red}}"))
			experiment.RecordValue("avg Runtime(ms)", float64(duration), gmeasure.Style("{{green}}"), gmeasure.Precision(time.Millisecond))
		})
	})
})

func propagateDeployment(deployment *appsv1.Deployment) *policyv1alpha1.PropagationPolicy {
	policy := testhelper.NewPropagationPolicy(deployment.Namespace, deployment.Name, []policyv1alpha1.ResourceSelector{
		{
			APIVersion: deployment.APIVersion,
			Kind:       deployment.Kind,
			Name:       deployment.Name,
		},
	}, policyv1alpha1.Placement{
		ClusterAffinity: &policyv1alpha1.ClusterAffinity{
			ClusterNames: framework.ClusterNames(),
		},
	})

	policy.Spec.Suspension = &policyv1alpha1.Suspension{Dispatching: ptr.To(true)}
	framework.CreatePropagationPolicy(karmadaClient, policy)
	framework.WaitDeploymentPresentOnClustersFitWith(framework.ClusterNames(), deployment.Namespace, deployment.Name, func(_ *appsv1.Deployment) bool {
		return true
	})
	return policy
}

func createDeployment(namespace, name string) *appsv1.Deployment {
	deployment := testhelper.NewDeployment(namespace, name)
	framework.CreateDeployment(kubeClient, deployment)
	return deployment
}

func applyOverridePolicy(deployment *appsv1.Deployment) *policyv1alpha1.OverridePolicy {
	overridePolicy := testhelper.NewOverridePolicyByOverrideRules(deployment.Namespace, deployment.Name, []policyv1alpha1.ResourceSelector{
		{
			APIVersion: deployment.APIVersion,
			Kind:       deployment.Kind,
			Name:       deployment.Name,
		},
	}, []policyv1alpha1.RuleWithCluster{
		{
			TargetCluster: &policyv1alpha1.ClusterAffinity{
				ClusterNames: []string{framework.ClusterNames()[0]},
			},
			Overriders: policyv1alpha1.Overriders{
				LabelsOverrider: []policyv1alpha1.LabelAnnotationOverrider{
					{
						Operator: policyv1alpha1.OverriderOpAdd,
						Value:    map[string]string{deployment.Name: deployment.Name},
					},
				},
			},
		},
	})
	framework.CreateOverridePolicy(karmadaClient, overridePolicy)
	framework.WaitDeploymentPresentOnClusterFitWith(framework.ClusterNames()[0], deployment.Namespace, deployment.Name, func(d *appsv1.Deployment) bool {
		return d.GetLabels()[deployment.Name] == deployment.Name
	})
	return overridePolicy
}

func getDurationMetric(grabber *framework.Grabber, sumMetricName, countMetricName, sampleName string) (
	sumMetric *model.Sample, countMetric *model.Sample) {
	ginkgo.By("fetch metrics", func() {
		metrics, err := grabber.GrabMetricsFromComponentLeader(context.TODO(), names.KarmadaControllerManagerComponentName)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		sumMetric = framework.GetMetricByName(metrics[sumMetricName], sampleName)
		countMetric = framework.GetMetricByName(metrics[countMetricName], sampleName)
		gomega.Expect(sumMetric).ShouldNot(gomega.BeNil())
		gomega.Expect(countMetric).ShouldNot(gomega.BeNil())
	})
	return sumMetric, countMetric
}

func waitWorkQueueFinished(grabber *framework.Grabber, depthMetricName, sampleName string) {
	time.Sleep(5 * time.Second)
	gomega.Eventually(func() bool {
		metrics, err := grabber.GrabMetricsFromComponentLeader(context.TODO(), names.KarmadaControllerManagerComponentName)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		addsTotalMetric := framework.GetMetricByName(metrics["workqueue_adds_total"], sampleName)
		gomega.Expect(addsTotalMetric).ShouldNot(gomega.BeNil())

		countMetric := framework.GetMetricByName(metrics["workqueue_work_duration_seconds_count"], sampleName)
		gomega.Expect(countMetric).ShouldNot(gomega.BeNil())

		return addsTotalMetric.Value == countMetric.Value
	}, pollTimeout, pollInterval).Should(gomega.Equal(true))
}
