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
	// ResourceKindReschedule is kind name of Reschedule.
	ResourceKindReschedule = "Reschedule"
	// ResourceSingularReschedule is singular name of Reschedule.
	ResourceSingularReschedule = "reschedule"
	// ResourcePluralReschedule is plural name of Reschedule.
	ResourcePluralReschedule = "reschedules"
	// ResourceNamespaceScopedReschedule indicates if Reschedule is NamespaceScoped.
	ResourceNamespaceScopedReschedule = false
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope="Cluster"

// Reschedule represents the desire state and status of a task which can enforces a rescheduling.
type Reschedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec represents the specification of the desired behavior of Reschedule.
	// +required
	Spec RescheduleSpec `json:"spec"`
}

// RescheduleSpec represents the specification of the desired behavior of Reschedule.
type RescheduleSpec struct {
	// TargetRefPolicy used to select batch of resources managed by certain policies.
	// +optional
	TargetRefPolicy []PolicySelector `json:"targetRefPolicy,omitempty"`

	// TargetRefResource used to select resources.
	// +optional
	TargetRefResource []ResourceSelector `json:"targetRefResource,omitempty"`
}

// PolicySelector the resources bound policy will be selected.
type PolicySelector struct {
	// Namespace of the target policy.
	// Default is empty, which means inherit from the parent object scope.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Name of the target resource.
	// Default is empty, which means selecting all resources.
	// +optional
	Name string `json:"name,omitempty"`
}

// ResourceSelector the resources will be selected.
type ResourceSelector struct {
	// APIVersion represents the API version of the target resources.
	// +required
	APIVersion string `json:"apiVersion"`

	// Kind represents the Kind of the target resources.
	// +required
	Kind string `json:"kind"`

	// Namespace of the target resource.
	// Default is empty, which means inherit from the parent object scope.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Name of the target resource.
	// Default is empty, which means selecting all resources.
	// +optional
	Name string `json:"name,omitempty"`

	// A label query over a set of resources.
	// If name is not empty, labelSelector will be ignored.
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RescheduleList contains a list of Reschedule
type RescheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items holds a list of Reschedule.
	Items []Reschedule `json:"items"`
}
