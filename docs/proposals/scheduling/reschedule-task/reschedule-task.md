---
title: Introduce a mechanism to actively triggle rescheduling
authors:
  - "@chaosi-zju"
reviewers:
  - "@RainbowMango"
  - "@chaunceyjiang"
  - "TBD"
approvers:
  - "@RainbowMango"
  - "TBD"

creation-date: 2024-01-30
---

# Introduce a mechanism to actively trigger rescheduling

## Background

According to the current implementation, after the replicas of workload is scheduled, it will remain inertia and the 
replicas distribution will not change. 

However, in some scenarios, users hope to have means to actively trigger rescheduling.

### Motivation

Assuming the user has propagated the workloads to member clusters, replicas migrated due to member cluster failure.

However, the user expects an approach to trigger rescheduling after member cluster restored, so that replicas can
migrate back.

### Goals

Introduce a mechanism to actively trigger rescheduling of workload resource.

### Applicable scenario

This feature might help in a scenario where: the `replicas` in resource template or `placement` in policy has not changed, 
but the user wants to actively trigger rescheduling of replicas.

## Proposal

### Overview

This proposal aims to introduce a mechanism of active triggering rescheduling, which benefits a lot in application 
failover scenarios. This can be realized by introducing a new API, and a new field would be marked when this new API 
called, so that scheduler can perceive the need for rescheduling.

### User story

In application failover scenarios, replicas migrated from primary cluster to backup cluster when primary cluster failue.

As a user, I want to trigger replicas migrating back when cluster restored, so that:

1. restore the disaster recovery mode to ensure the reliability and stability of the cluster.
2. save the cost of the backup cluster.

### Notes/Constraints/Caveats

This ability is limited to triggering rescheduling. The scheduling result will be recalculated according to the
Placement in the current ResourceBinding, and the scheduling result is not guaranteed to be exactly the same as before
the cluster failure.

> Notes: pay attention to the recalculation is basing on Placement in the current `ResourceBinding`, not "Policy". So if
> your activation preference of Policy is `Lazy`, the rescheduling is still basing on previous `ResourceBinding` even if
> the current Policy has been changed.

## Design Details

### API change

* Introduce a new API named `WorkloadRebalancer` into a new apiGroup `apps.karmada.io/v1alpha1`:

```go
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
    Success ObservedState = "Success"
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
```

* Add two new fields to ResourceBinding/ClusterResourceBinding, one is in `spec` named `rescheduleTriggeredAt`, another
is in `status` named `lastScheduledTime`, detail description is as follows:

```go
// ResourceBindingSpec represents the expectation of ResourceBinding.
type ResourceBindingSpec struct {
    ...
	// RescheduleTriggeredAt is a timestamp representing when the referenced resource is triggered rescheduling.
	// When this field is updated, it means a rescheduling is manually triggered by user, and the expected behavior
	// of this action is to do a complete recalculation without referring to last scheduling results.
	// It works with the status.lastScheduledTime field, and only when this timestamp is later than timestamp in
	// status.lastScheduledTime will the rescheduling actually execute, otherwise, ignored.
	//
	// It is represented in RFC3339 form (like '2006-01-02T15:04:05Z') and is in UTC.
	// +optional
	RescheduleTriggeredAt *metav1.Time `json:"rescheduleTriggeredAt,omitempty"`
    ...
}

// ResourceBindingStatus represents the overall status of the strategy as well as the referenced resources.
type ResourceBindingStatus struct {
	...
	// LastScheduledTime representing the latest timestamp when scheduler successfully finished a scheduling.
	// It is represented in RFC3339 form (like '2006-01-02T15:04:05Z') and is in UTC.
	// +optional
	LastScheduledTime *metav1.Time `json:"lastScheduledTime,omitempty"`
    ...
}
```

### Example

Assuming there is a Deployment named `nginx`, the user wants to trigger its rescheduling,
he just needs to apply following yaml:

```yaml
apiVersion: apps.karmada.io/v1alpha1
kind: WorkloadRebalancer
metadata:
  name: demo
spec:
  workloads:
    - apiVersion: apps/v1
      kind: Deployment
      name: demo-test-1
      namespace: default
    - apiVersion: apps/v1
      kind: Deployment
      name: demo-test-2
      namespace: default
```

> Notes: as for `workloads` field:
> 1. `name` sub-field is required;
> 2. `namespace` sub-field is required when it is a namespace scoped resource, while empty when it is a cluster wide resource;

Then, he will get a `workloadrebalancer.apps.karmada.io/demo created` result, which means the API created success, attention,
not finished. Simultaneously, he will see the new field `spec.placement.rescheduleTriggeredAt` in binding of the selected
resource been set to the `CreationTimestamp` of `workloadrebalancer/demo`.

```yaml
apiVersion: work.karmada.io/v1alpha2
kind: ResourceBinding
metadata:
  name: nginx-deployment
  namespace: default
spec:
  rescheduleTriggeredAt: "2024-04-17T15:04:05Z"
  ...
```

Then, rescheduling is in progress. If it succeeds, the `status.lastScheduledTime` field of binding will be updated,
which represents scheduler finished a rescheduling.; If it failed, scheduler will retry.

```yaml
apiVersion: work.karmada.io/v1alpha2
kind: ResourceBinding
metadata:
  name: nginx-deployment
  namespace: default
spec:
  rescheduleTriggeredAt: "2024-04-17T15:04:05Z"
  ...
status:
  lastScheduledTime: "2024-04-17T15:04:06Z"
  conditions:
    - ...
    - lastTransitionTime: "2024-03-08T08:53:03Z"
      message: Binding has been scheduled successfully.
      reason: Success
      status: "True"
      type: Scheduled
    - lastTransitionTime: "2024-03-08T08:53:03Z"
      message: All works have been successfully applied
      reason: FullyAppliedSuccess
      status: "True"
      type: FullyApplied
```

Finally, all works have been successfully applied, the user will observe changes in the actual distribution of resource 
template; the user can also see several recorded event in resource template, just like:

```shell
$ kubectl --context karmada-apiserver describe deployment demo
...
Events:
  Type    Reason                  Age                From                                Message
  ----    ------                  ----               ----                                -------
  ...
  Normal  ScheduleBindingSucceed  31s                default-scheduler                   Binding has been scheduled successfully.
  Normal  GetDependenciesSucceed  31s                dependencies-distributor            Get dependencies([]) succeed.
  Normal  SyncSucceed             31s                execution-controller                Successfully applied resource(default/demo) to cluster member1
  Normal  AggregateStatusSucceed  31s (x4 over 31s)  resource-binding-status-controller  Update resourceBinding(default/demo-deployment) with AggregatedStatus successfully.
  Normal  SyncSucceed             31s                execution-controller                Successfully applied resource(default/demo1) to cluster member2
```

besides, the user can observe the rebalance result at `status.observedWorkloads` of `workloadrebalancer/demo`, just like:

```yaml
apiVersion: apps.karmada.io/v1alpha1
kind: WorkloadRebalancer
metadata:
 name: demo
spec:
  workloads:
    - apiVersion: apps/v1
      kind: Deployment
      name: demo-test-1
      namespace: default
    - apiVersion: apps/v1
      kind: Deployment
      name: demo-test-2
      namespace: default
status:
  observedWorkloads:
    - apiVersion: apps/v1
      kind: Deployment
      name: demo-test-1
      namespace: default
      state: Success
    - apiVersion: apps/v1
      kind: Deployment
      name: demo-test-2
      namespace: default
      state: Failed
      reason: NotFound
```

### Implementation logic

1) add an CRD type API named `WorkloadRebalancer`, detail described as above.

2) add a controller into controller-manager, which will fetch all referred resource declared in `workloads`, 
and then set `spec.rescheduleTriggeredAt` field to current timestamp in corresponding ResourceBinding.

3) in scheduling process, add a trigger condition: even if `Placement` and `Replicas` of binding unchanged, schedule will
be triggerred if `spec.rescheduleTriggeredAt` is later than `status.lastScheduledTime`. After schedule finished, scheduler 
will update `status.lastScheduledTime` when refreshing binding back.
