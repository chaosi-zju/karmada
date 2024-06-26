//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright The Karmada Authors.

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

// Code generated by deepcopy-gen. DO NOT EDIT.

package v1alpha1

import (
	v2 "k8s.io/api/autoscaling/v2"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterMetric) DeepCopyInto(out *ClusterMetric) {
	*out = *in
	in.MetricTimestamp.DeepCopyInto(&out.MetricTimestamp)
	if in.AverageRequest != nil {
		in, out := &in.AverageRequest, &out.AverageRequest
		*out = new(int64)
		**out = **in
	}
	if in.MissingPodAverageRequest != nil {
		in, out := &in.MissingPodAverageRequest, &out.MissingPodAverageRequest
		*out = new(int64)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterMetric.
func (in *ClusterMetric) DeepCopy() *ClusterMetric {
	if in == nil {
		return nil
	}
	out := new(ClusterMetric)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CronFederatedHPA) DeepCopyInto(out *CronFederatedHPA) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CronFederatedHPA.
func (in *CronFederatedHPA) DeepCopy() *CronFederatedHPA {
	if in == nil {
		return nil
	}
	out := new(CronFederatedHPA)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CronFederatedHPA) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CronFederatedHPAList) DeepCopyInto(out *CronFederatedHPAList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CronFederatedHPA, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CronFederatedHPAList.
func (in *CronFederatedHPAList) DeepCopy() *CronFederatedHPAList {
	if in == nil {
		return nil
	}
	out := new(CronFederatedHPAList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CronFederatedHPAList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CronFederatedHPARule) DeepCopyInto(out *CronFederatedHPARule) {
	*out = *in
	if in.TargetReplicas != nil {
		in, out := &in.TargetReplicas, &out.TargetReplicas
		*out = new(int32)
		**out = **in
	}
	if in.TargetMinReplicas != nil {
		in, out := &in.TargetMinReplicas, &out.TargetMinReplicas
		*out = new(int32)
		**out = **in
	}
	if in.TargetMaxReplicas != nil {
		in, out := &in.TargetMaxReplicas, &out.TargetMaxReplicas
		*out = new(int32)
		**out = **in
	}
	if in.Suspend != nil {
		in, out := &in.Suspend, &out.Suspend
		*out = new(bool)
		**out = **in
	}
	if in.TimeZone != nil {
		in, out := &in.TimeZone, &out.TimeZone
		*out = new(string)
		**out = **in
	}
	if in.SuccessfulHistoryLimit != nil {
		in, out := &in.SuccessfulHistoryLimit, &out.SuccessfulHistoryLimit
		*out = new(int32)
		**out = **in
	}
	if in.FailedHistoryLimit != nil {
		in, out := &in.FailedHistoryLimit, &out.FailedHistoryLimit
		*out = new(int32)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CronFederatedHPARule.
func (in *CronFederatedHPARule) DeepCopy() *CronFederatedHPARule {
	if in == nil {
		return nil
	}
	out := new(CronFederatedHPARule)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CronFederatedHPASpec) DeepCopyInto(out *CronFederatedHPASpec) {
	*out = *in
	out.ScaleTargetRef = in.ScaleTargetRef
	if in.Rules != nil {
		in, out := &in.Rules, &out.Rules
		*out = make([]CronFederatedHPARule, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CronFederatedHPASpec.
func (in *CronFederatedHPASpec) DeepCopy() *CronFederatedHPASpec {
	if in == nil {
		return nil
	}
	out := new(CronFederatedHPASpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CronFederatedHPAStatus) DeepCopyInto(out *CronFederatedHPAStatus) {
	*out = *in
	if in.ExecutionHistories != nil {
		in, out := &in.ExecutionHistories, &out.ExecutionHistories
		*out = make([]ExecutionHistory, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CronFederatedHPAStatus.
func (in *CronFederatedHPAStatus) DeepCopy() *CronFederatedHPAStatus {
	if in == nil {
		return nil
	}
	out := new(CronFederatedHPAStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ExecutionHistory) DeepCopyInto(out *ExecutionHistory) {
	*out = *in
	if in.NextExecutionTime != nil {
		in, out := &in.NextExecutionTime, &out.NextExecutionTime
		*out = (*in).DeepCopy()
	}
	if in.SuccessfulExecutions != nil {
		in, out := &in.SuccessfulExecutions, &out.SuccessfulExecutions
		*out = make([]SuccessfulExecution, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.FailedExecutions != nil {
		in, out := &in.FailedExecutions, &out.FailedExecutions
		*out = make([]FailedExecution, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ExecutionHistory.
func (in *ExecutionHistory) DeepCopy() *ExecutionHistory {
	if in == nil {
		return nil
	}
	out := new(ExecutionHistory)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FailedExecution) DeepCopyInto(out *FailedExecution) {
	*out = *in
	if in.ScheduleTime != nil {
		in, out := &in.ScheduleTime, &out.ScheduleTime
		*out = (*in).DeepCopy()
	}
	if in.ExecutionTime != nil {
		in, out := &in.ExecutionTime, &out.ExecutionTime
		*out = (*in).DeepCopy()
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FailedExecution.
func (in *FailedExecution) DeepCopy() *FailedExecution {
	if in == nil {
		return nil
	}
	out := new(FailedExecution)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FederatedHPA) DeepCopyInto(out *FederatedHPA) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FederatedHPA.
func (in *FederatedHPA) DeepCopy() *FederatedHPA {
	if in == nil {
		return nil
	}
	out := new(FederatedHPA)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *FederatedHPA) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FederatedHPAClusterStatus) DeepCopyInto(out *FederatedHPAClusterStatus) {
	*out = *in
	if in.ClusterMetric != nil {
		in, out := &in.ClusterMetric, &out.ClusterMetric
		*out = make([]*ClusterMetric, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(ClusterMetric)
				(*in).DeepCopyInto(*out)
			}
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FederatedHPAClusterStatus.
func (in *FederatedHPAClusterStatus) DeepCopy() *FederatedHPAClusterStatus {
	if in == nil {
		return nil
	}
	out := new(FederatedHPAClusterStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FederatedHPAList) DeepCopyInto(out *FederatedHPAList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]FederatedHPA, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FederatedHPAList.
func (in *FederatedHPAList) DeepCopy() *FederatedHPAList {
	if in == nil {
		return nil
	}
	out := new(FederatedHPAList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *FederatedHPAList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FederatedHPASpec) DeepCopyInto(out *FederatedHPASpec) {
	*out = *in
	out.ScaleTargetRef = in.ScaleTargetRef
	if in.MinReplicas != nil {
		in, out := &in.MinReplicas, &out.MinReplicas
		*out = new(int32)
		**out = **in
	}
	if in.Metrics != nil {
		in, out := &in.Metrics, &out.Metrics
		*out = make([]v2.MetricSpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Behavior != nil {
		in, out := &in.Behavior, &out.Behavior
		*out = new(v2.HorizontalPodAutoscalerBehavior)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FederatedHPASpec.
func (in *FederatedHPASpec) DeepCopy() *FederatedHPASpec {
	if in == nil {
		return nil
	}
	out := new(FederatedHPASpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FederatedHPAStatus) DeepCopyInto(out *FederatedHPAStatus) {
	*out = *in
	in.HorizontalPodAutoscalerStatus.DeepCopyInto(&out.HorizontalPodAutoscalerStatus)
	if in.ClusterStatus != nil {
		in, out := &in.ClusterStatus, &out.ClusterStatus
		*out = make(map[string]FederatedHPAClusterStatus, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FederatedHPAStatus.
func (in *FederatedHPAStatus) DeepCopy() *FederatedHPAStatus {
	if in == nil {
		return nil
	}
	out := new(FederatedHPAStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SuccessfulExecution) DeepCopyInto(out *SuccessfulExecution) {
	*out = *in
	if in.ScheduleTime != nil {
		in, out := &in.ScheduleTime, &out.ScheduleTime
		*out = (*in).DeepCopy()
	}
	if in.ExecutionTime != nil {
		in, out := &in.ExecutionTime, &out.ExecutionTime
		*out = (*in).DeepCopy()
	}
	if in.AppliedReplicas != nil {
		in, out := &in.AppliedReplicas, &out.AppliedReplicas
		*out = new(int32)
		**out = **in
	}
	if in.AppliedMaxReplicas != nil {
		in, out := &in.AppliedMaxReplicas, &out.AppliedMaxReplicas
		*out = new(int32)
		**out = **in
	}
	if in.AppliedMinReplicas != nil {
		in, out := &in.AppliedMinReplicas, &out.AppliedMinReplicas
		*out = new(int32)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SuccessfulExecution.
func (in *SuccessfulExecution) DeepCopy() *SuccessfulExecution {
	if in == nil {
		return nil
	}
	out := new(SuccessfulExecution)
	in.DeepCopyInto(out)
	return out
}
