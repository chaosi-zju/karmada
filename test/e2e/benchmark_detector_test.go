/*
Copyright 2020 The Karmada Authors.

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
	"runtime"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	"github.com/prometheus/common/model"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/component-base/metrics/testutil"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	"github.com/karmada-io/karmada/test/e2e/framework"
	testhelper "github.com/karmada-io/karmada/test/helper"
)

var _ = framework.SerialDescribe("detector benchmark testing", func() {
	N := 10
	var deployments []*appsv1.Deployment
	var policies []*policyv1alpha1.PropagationPolicy
	var grabber *testhelper.Grabber

	const (
		resourceMatchPolicySumMetric   = "resource_match_policy_duration_seconds_sum"
		resourceMatchPolicyCountMetric = "resource_match_policy_duration_seconds_count"
		policyUpdateSumMetric          = "workqueue_work_duration_seconds_sum"
		policyUpdateCountMetric        = "workqueue_work_duration_seconds_count"
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
			policy.Spec.Suspension = &policyv1alpha1.Suspension{Dispatching: ptr.To(true)}

			deployments = append(deployments, deployment)
			policies = append(policies, policy)
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

		framework.WaitResourceBindingListFitWith(karmadaClient, testNamespace, func(list *workv1alpha2.ResourceBindingList) bool { return len(list.Items) >= N })
	})

	ginkgo.Context("resource reconcile benchmark testing", func() {
		var startMetrics *testutil.Metrics
		var endMetrics *testutil.Metrics

		ginkgo.BeforeEach(func() {
			var err error
			startMetrics, err = grabber.GrabMetricsFromKarmadaControllerManager(context.TODO())
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			gomega.Expect((*startMetrics)[resourceMatchPolicySumMetric].Len()).Should(gomega.Equal(1))
			gomega.Expect((*startMetrics)[resourceMatchPolicyCountMetric].Len()).Should(gomega.Equal(1))
		})

		ginkgo.It("resource match policy", func() {
			ginkgo.By("1. fetch final metrics", func() {
				var err error
				endMetrics, err = grabber.GrabMetricsFromKarmadaControllerManager(context.TODO())
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
				gomega.Expect((*endMetrics)[resourceMatchPolicySumMetric].Len()).Should(gomega.Equal(1))
				gomega.Expect((*endMetrics)[resourceMatchPolicyCountMetric].Len()).Should(gomega.Equal(1))
			})

			ginkgo.By("2. calculate time cost", func() {
				secondsSum := (*endMetrics)[resourceMatchPolicySumMetric][0].Value - (*startMetrics)[resourceMatchPolicySumMetric][0].Value
				secondsCnt := (*endMetrics)[resourceMatchPolicyCountMetric][0].Value - (*startMetrics)[resourceMatchPolicyCountMetric][0].Value

				klog.Infof("per resource match policy cost: %.3f", secondsSum/secondsCnt)
				gomega.Expect(secondsSum / secondsCnt).Should(gomega.BeNumerically("<", 0.2))
			})
		})

	})

	ginkgo.Context("policy update benchmark testing", func() {
		ginkgo.It("policy update", func() {
			var startSumMetric, startCountMetric *model.Sample
			var endSumMetric, endCountMetric *model.Sample

			ginkgo.By("1. fetch start metrics", func() {
				startMetrics, err := grabber.GrabMetricsFromKarmadaControllerManager(context.TODO())
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				startSumMetric = testhelper.GetMetricByName((*startMetrics)[policyUpdateSumMetric], "propagationPolicy reconciler")
				startCountMetric = testhelper.GetMetricByName((*startMetrics)[policyUpdateCountMetric], "propagationPolicy reconciler")
				gomega.Expect(startSumMetric).ShouldNot(gomega.BeNil())
				gomega.Expect(startCountMetric).ShouldNot(gomega.BeNil())
			})

			ginkgo.By("2. update policy", func() {
				for i := 0; i < N; i++ {
					policies[i].Spec.ConflictResolution = policyv1alpha1.ConflictOverwrite
					framework.UpdatePropagationPolicyWithSpec(karmadaClient, policies[i].Namespace, policies[i].Name, policies[i].Spec)
				}
			})

			ginkgo.By("3. fetch end metrics", func() {
				endMetrics, err := grabber.GrabMetricsFromKarmadaControllerManager(context.TODO())
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				endSumMetric = testhelper.GetMetricByName((*endMetrics)[policyUpdateSumMetric], "propagationPolicy reconciler")
				endCountMetric = testhelper.GetMetricByName((*endMetrics)[policyUpdateCountMetric], "propagationPolicy reconciler")
				gomega.Expect(startSumMetric).ShouldNot(gomega.BeNil())
				gomega.Expect(startCountMetric).ShouldNot(gomega.BeNil())
			})

			ginkgo.By("4. calculate time cost", func() {
				secondsSum := endSumMetric.Value - startSumMetric.Value
				secondsCnt := endCountMetric.Value - startCountMetric.Value

				klog.Infof("(*startMetrics)sum.Value: %.3f", startSumMetric.Value)
				klog.Infof("(*endMetrics)sum.Value: %.3f", endSumMetric.Value)
				klog.Infof("(*startMetrics)count.Value: %.3f", startCountMetric.Value)
				klog.Infof("(*endMetrics)count.Value: %.3f", endCountMetric.Value)
				klog.Infof("per resource match policy cost: %.3f", secondsSum/secondsCnt)
				//gomega.Expect(secondsSum / secondsCnt).Should(gomega.BeNumerically("<", 0.2))
			})
		})
	})

	ginkgo.Context("policy delete benchmark testing", func() {
		ginkgo.It("policy delete", func() {
			experiment := gmeasure.NewExperiment("policy delete performance")
			ginkgo.AddReportEntry(experiment.Name, experiment)

			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			experiment.RecordValue("Memory Usage", float64(m.Alloc/1024/1024), gmeasure.Style("{{red}}"), gmeasure.Precision(3), gmeasure.Units("MB"))

			experiment.Sample(func(idx int) {
				startSumMetric, startCountMetric := getDurationMetric(grabber, policyUpdateSumMetric, policyUpdateCountMetric, "propagationPolicy reconciler")
				framework.RemovePropagationPolicyIfExist(karmadaClient, policies[idx].Namespace, policies[idx].Name)
				endSumMetric, endCountMetric := getDurationMetric(grabber, policyUpdateSumMetric, policyUpdateCountMetric, "propagationPolicy reconciler")

				secondsSum := endSumMetric.Value - startSumMetric.Value
				secondsCnt := endCountMetric.Value - startCountMetric.Value
				duration := secondsSum / secondsCnt

				experiment.RecordValue("Runtime", float64(duration), gmeasure.Style("{{green}}"), gmeasure.Precision(time.Microsecond), gmeasure.Annotation(fmt.Sprintf("%d", idx)))

			}, gmeasure.SamplingConfig{N: 100, Duration: 50 * time.Millisecond, NumParallel: 2})
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
