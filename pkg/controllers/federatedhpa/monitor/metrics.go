/*
Copyright 2023 The Kubernetes Authors.

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

// Package monitor contains metrics which are exposed from the HPA controller.
package monitor

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	// hpaControllerSubsystem - subsystem name used by HPA controller
	hpaControllerSubsystem = "horizontal_pod_autoscaler_controller"
)

var (
	reconciliationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: hpaControllerSubsystem,
			Name:      "reconciliations_total",
			Help:      "Number of reconciliations of HPA controller. The label 'action' should be either 'scale_down', 'scale_up', or 'none'. Also, the label 'error' should be either 'spec', 'internal', or 'none'. Note that if both spec and internal errors happen during a reconciliation, the first one to occur is reported in `error` label.",
		}, []string{"action", "error"})

	reconciliationsDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: hpaControllerSubsystem,
			Name:      "reconciliation_duration_seconds",
			Help:      "The time(seconds) that the HPA controller takes to reconcile once. The label 'action' should be either 'scale_down', 'scale_up', or 'none'. Also, the label 'error' should be either 'spec', 'internal', or 'none'. Note that if both spec and internal errors happen during a reconciliation, the first one to occur is reported in `error` label.",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15),
		}, []string{"action", "error"})
	metricComputationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: hpaControllerSubsystem,
			Name:      "metric_computation_total",
			Help:      "Number of metric computations. The label 'action' should be either 'scale_down', 'scale_up', or 'none'. Also, the label 'error' should be either 'spec', 'internal', or 'none'. The label 'metric_type' corresponds to HPA.spec.metrics[*].type",
		}, []string{"action", "error", "metric_type"})
	metricComputationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: hpaControllerSubsystem,
			Name:      "metric_computation_duration_seconds",
			Help:      "The time(seconds) that the HPA controller takes to calculate one metric. The label 'action' should be either 'scale_down', 'scale_up', or 'none'. The label 'error' should be either 'spec', 'internal', or 'none'. The label 'metric_type' corresponds to HPA.spec.metrics[*].type",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15),
		}, []string{"action", "error", "metric_type"})

	metricsList = []prometheus.Collector{
		reconciliationsTotal,
		reconciliationsDuration,
		metricComputationTotal,
		metricComputationDuration,
	}
)

var register sync.Once

// Register all metrics.
func Register() {
	// Register the metrics.
	register.Do(func() {
		registerMetrics(metricsList...)
	})
}

// RegisterMetrics registers a list of metrics.
func registerMetrics(extraMetrics ...prometheus.Collector) {
	for _, metric := range extraMetrics {
		ctrlmetrics.Registry.MustRegister(metric)
	}
}
