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

	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	"github.com/karmada-io/karmada/test/e2e/framework"
	testhelper "github.com/karmada-io/karmada/test/helper"
)

var _ = framework.SerialDescribe("detector benchmark testing", func() {
	N := 10
	var deployments []*appsv1.Deployment
	var policies []*policyv1alpha1.PropagationPolicy
	var overridePolicies []*policyv1alpha1.OverridePolicy
	var grabber *testhelper.Grabber

	const (
		workDurationSumMetric   = "workqueue_work_duration_seconds_sum"
		workDurationCountMetric = "workqueue_work_duration_seconds_count"
	)

	ginkgo.BeforeEach(func() {
		var err error
		grabber, err = testhelper.NewMetricsGrabber(context.TODO(), hostKubeClient)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		for i := 0; i < N; i++ {
			deployment := testhelper.NewDeployment(testNamespace, fmt.Sprintf("%s-%s-%d", deploymentNamePrefix, rand.String(RandomStrLength), i+1))
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
								Value:    map[string]string{"test": "test"},
							},
						},
					},
				},
			})

			deployments = append(deployments, deployment)
			policies = append(policies, policy)
			overridePolicies = append(overridePolicies, overridePolicy)
		}
	})

	ginkgo.JustBeforeEach(func() {
		for i := 0; i < N; i++ {
			framework.CreatePropagationPolicy(karmadaClient, policies[i])
			framework.CreateDeployment(kubeClient, deployments[i])
		}

		ginkgo.DeferCleanup(func() {
			for i := 0; i < N; i++ {
				framework.RemovePropagationPolicyIfExist(karmadaClient, policies[i].Namespace, policies[i].Name)
				framework.RemoveDeployment(kubeClient, deployments[i].Namespace, deployments[i].Name)
			}
		})

		for i := 0; i < N; i++ {
			framework.WaitDeploymentPresentOnClustersFitWith(framework.ClusterNames(), deployments[i].Namespace, deployments[i].Name, func(_ *appsv1.Deployment) bool {
				return true
			})
		}
	})

	ginkgo.Context("syncWork benchmark testing", func() {
		ginkgo.It("syncWork", func() {
			experiment := gmeasure.NewExperiment("syncWork performance")
			ginkgo.AddReportEntry(experiment.Name, experiment)

			experiment.Sample(func(idx int) {
				startSumMetric, startCountMetric := getDurationMetric(grabber, workDurationSumMetric, workDurationCountMetric, "binding-controller")

				i := rand.Intn(N)
				overridePolicies[i].Spec.OverrideRules[0].Overriders.LabelsOverrider[0].Value = map[string]string{overridePolicies[i].Name: "test"}
				framework.CreateOverridePolicy(karmadaClient, overridePolicies[i])
				framework.WaitDeploymentPresentOnClusterFitWith(framework.ClusterNames()[0], deployments[i].Namespace, deployments[i].Name, func(d *appsv1.Deployment) bool {
					return d.GetLabels()[overridePolicies[i].Name] == "test"
				})

				endSumMetric, endCountMetric := getDurationMetric(grabber, workDurationSumMetric, workDurationCountMetric, "binding-controller")

				secondsSum := endSumMetric.Value - startSumMetric.Value
				secondsCnt := endCountMetric.Value - startCountMetric.Value
				duration := secondsSum / secondsCnt

				experiment.RecordValue("Runtime", float64(duration), gmeasure.Style("{{green}}"), gmeasure.Precision(time.Microsecond), gmeasure.Annotation(fmt.Sprintf("%d", idx)))

			}, gmeasure.SamplingConfig{N: 10, Duration: 50 * time.Millisecond, NumParallel: 2})
		})
	})
})

func getDurationMetric(grabber *testhelper.Grabber, sumMetricName, countMetricName, sampleName string) (
	sumMetric *model.Sample, countMetric *model.Sample) {
	ginkgo.By("fetch metrics", func() {
		metrics, err := grabber.GrabMetricsFromKarmadaControllerManager(context.TODO())
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		sumMetric = testhelper.GetMetricByName((*metrics)[sumMetricName], sampleName)
		countMetric = testhelper.GetMetricByName((*metrics)[countMetricName], sampleName)
		gomega.Expect(sumMetric).ShouldNot(gomega.BeNil())
		gomega.Expect(countMetric).ShouldNot(gomega.BeNil())
	})
	return sumMetric, countMetric
}
