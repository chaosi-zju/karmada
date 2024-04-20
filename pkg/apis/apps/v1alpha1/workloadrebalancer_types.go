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

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	// ResourceKindWorkloadRebalancer is kind name of WorkloadRebalancer.
	ResourceKindWorkloadRebalancer = "WorkloadRebalancer"
	// ResourceSingularWorkloadRebalancer is singular name of WorkloadRebalancer.
	ResourceSingularWorkloadRebalancer = "workloadrebalancer"
	// ResourcePluralWorkloadRebalancer is kind plural name of WorkloadRebalancer.
	ResourcePluralWorkloadRebalancer = "workloadrebalancers"
	// ResourceNamespaceScopedWorkloadRebalancer indicates if WorkloadRebalancer is NamespaceScoped.
	ResourceNamespaceScopedWorkloadRebalancer = false
)

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=workloadrebalancers,scope="Cluster",shortName=wr,categories={karmada-io}
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WorkloadRebalancer represents the desired behavior and status of a job which can enforces a rescheduling.
//
// Notes: make sure the clocks of controller-manager and scheduler are synchronized when using this API.
type WorkloadRebalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec represents the specification of the desired behavior of WorkloadRebalancer.
	// +required
	Spec WorkloadRebalancerSpec `json:"spec"`

	// Status represents the status of WorkloadRebalancer.
	// +optional
	Status WorkloadRebalancerStatus `json:"status,omitempty"`
}

// WorkloadRebalancerSpec represents the specification of the desired behavior of Reschedule.
//
// Notes: this API represents a one-time execution process, once the object is created, the execution process begins,
// and it will not respond to any modification of the spec field.
type WorkloadRebalancerSpec struct {
	// Workloads used to specify the list of expected resource.
	// Nil or empty list is not allowed.
	// +kubebuilder:validation:MinItems=1
	// +required
	Workloads []Workload `json:"workloads"`
}

// Workload the expected resource.
type Workload struct {
	// APIVersion represents the API version of the target resource.
	// +required
	APIVersion string `json:"apiVersion"`

	// Kind represents the Kind of the target resource.
	// +required
	Kind string `json:"kind"`

	// Name of the target resource.
	// +required
	Name string `json:"name"`

	// Namespace of the target resource.
	// Default is empty, which means it is a non-namespacescoped resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// WorkloadRebalancerStatus contains information about the current status of a WorkloadRebalancer
// updated periodically by schedule trigger controller.
type WorkloadRebalancerStatus struct {
	// ObservedWorkloads contains information about the execution states and messages of target resources.
	// +optional
	ObservedWorkloads []ObservedWorkload `json:"observedWorkloads,omitempty"`
}

// ObservedWorkload the observed resource.
type ObservedWorkload struct {
	Workload `json:",inline"`

	// State the observed state of resource.
	// +optional
	State ObservedState `json:"state,omitempty"`

	// Reason represents a machine-readable description of why this workload failed.
	Reason metav1.StatusReason `json:"reason,omitempty"`
}

// ObservedState the specific extent to which the resource has been executed
type ObservedState string

const (
	// Failed the resource has been triggered a rescheduling failed.
	Failed ObservedState = "Failed"
	// Success the resource has been triggered a scheduling success.
	Success ObservedState = "Failed"
)

// +kubebuilder:resource:scope="Cluster"
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WorkloadRebalancerList contains a list of WorkloadRebalancer
type WorkloadRebalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items holds a list of WorkloadRebalancer.
	Items []WorkloadRebalancer `json:"items"`
}
