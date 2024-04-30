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

package workloadrebalancer

import (
	"context"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/karmada-io/karmada/pkg/apis/apps/v1alpha1"
	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	"github.com/karmada-io/karmada/pkg/util/gclient"
	"github.com/karmada-io/karmada/test/helper"
)

var (
	now        = metav1.Now().Rfc3339Copy()
	oneHourAgo = metav1.NewTime(time.Now().Add(-1 * time.Hour)).Rfc3339Copy()
	deploy1    = helper.NewDeployment("test-ns", "test-1")
	binding1   = &workv1alpha2.ResourceBinding{
		TypeMeta:   metav1.TypeMeta{Kind: "work.karmada.io/v1alpha2", APIVersion: "ResourceBinding"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "test-1-deployment"},
		Spec:       workv1alpha2.ResourceBindingSpec{RescheduleTriggeredAt: &oneHourAgo},
		Status:     workv1alpha2.ResourceBindingStatus{LastScheduledTime: &oneHourAgo},
	}
	deploy1Obj = appsv1alpha1.ObjectReference{
		APIVersion: deploy1.APIVersion,
		Kind:       deploy1.Kind,
		Name:       deploy1.Name,
		Namespace:  deploy1.Namespace,
	}

	pendingRebalancer = &appsv1alpha1.WorkloadRebalancer{
		ObjectMeta: metav1.ObjectMeta{Name: "pending-rebalancer", CreationTimestamp: now},
		Spec: appsv1alpha1.WorkloadRebalancerSpec{
			Workloads: []appsv1alpha1.ObjectReference{deploy1Obj},
		},
	}
	succeedRebalancer = &appsv1alpha1.WorkloadRebalancer{
		ObjectMeta: metav1.ObjectMeta{Name: "succeed-rebalancer", CreationTimestamp: oneHourAgo},
		Spec: appsv1alpha1.WorkloadRebalancerSpec{
			Workloads: []appsv1alpha1.ObjectReference{deploy1Obj},
		},
		Status: appsv1alpha1.WorkloadRebalancerStatus{
			ObservedWorkloads: []appsv1alpha1.ObservedWorkload{
				{
					Workload: deploy1Obj,
					Result:   appsv1alpha1.RebalanceSuccessful,
				},
			},
		},
	}
	notFoundRebalancer = &appsv1alpha1.WorkloadRebalancer{
		ObjectMeta: metav1.ObjectMeta{Name: "not-found-rebalancer", CreationTimestamp: now},
		Spec: appsv1alpha1.WorkloadRebalancerSpec{
			Workloads: []appsv1alpha1.ObjectReference{deploy1Obj},
		},
		Status: appsv1alpha1.WorkloadRebalancerStatus{
			ObservedWorkloads: []appsv1alpha1.ObservedWorkload{
				{
					Workload: deploy1Obj,
					Result:   appsv1alpha1.RebalanceFailed,
					Reason:   appsv1alpha1.RebalanceObjectNotFound,
				},
			},
		},
	}
	failedRebalancer = &appsv1alpha1.WorkloadRebalancer{
		ObjectMeta: metav1.ObjectMeta{Name: "failed-rebalancer", CreationTimestamp: now},
		Spec: appsv1alpha1.WorkloadRebalancerSpec{
			Workloads: []appsv1alpha1.ObjectReference{deploy1Obj},
		},
		Status: appsv1alpha1.WorkloadRebalancerStatus{
			ObservedWorkloads: []appsv1alpha1.ObservedWorkload{
				{
					Workload: deploy1Obj,
					Result:   appsv1alpha1.RebalanceFailed,
				},
			},
		},
	}
)

func TestRebalancerController_Reconcile(t *testing.T) {
	tests := []struct {
		name               string
		req                controllerruntime.Request
		wantErr            bool
		wantStatus         appsv1alpha1.WorkloadRebalancerStatus
		wantRescheduleTime *metav1.Time
	}{
		{
			name: "reconcile pendingRebalancer",
			req: controllerruntime.Request{
				NamespacedName: types.NamespacedName{Name: pendingRebalancer.Name},
			},
			wantStatus: appsv1alpha1.WorkloadRebalancerStatus{
				ObservedWorkloads: []appsv1alpha1.ObservedWorkload{
					{
						Workload: deploy1Obj,
						Result:   appsv1alpha1.RebalanceSuccessful,
					},
				},
			},
			wantRescheduleTime: &now,
		},
		{
			name: "reconcile succeedRebalancer",
			req: controllerruntime.Request{
				NamespacedName: types.NamespacedName{Name: succeedRebalancer.Name},
			},
			wantStatus: appsv1alpha1.WorkloadRebalancerStatus{
				ObservedWorkloads: []appsv1alpha1.ObservedWorkload{
					{
						Workload: deploy1Obj,
						Result:   appsv1alpha1.RebalanceSuccessful,
					},
				},
			},
			wantRescheduleTime: &oneHourAgo,
		},
		{
			name: "reconcile notFoundRebalancer",
			req: controllerruntime.Request{
				NamespacedName: types.NamespacedName{Name: notFoundRebalancer.Name},
			},
			wantStatus: appsv1alpha1.WorkloadRebalancerStatus{
				ObservedWorkloads: []appsv1alpha1.ObservedWorkload{
					{
						Workload: deploy1Obj,
						Result:   appsv1alpha1.RebalanceFailed,
						Reason:   appsv1alpha1.RebalanceObjectNotFound,
					},
				},
			},
			wantRescheduleTime: &oneHourAgo,
		},
		{
			name: "reconcile failedRebalancer",
			req: controllerruntime.Request{
				NamespacedName: types.NamespacedName{Name: failedRebalancer.Name},
			},
			wantStatus: appsv1alpha1.WorkloadRebalancerStatus{
				ObservedWorkloads: []appsv1alpha1.ObservedWorkload{
					{
						Workload: deploy1Obj,
						Result:   appsv1alpha1.RebalanceSuccessful,
					},
				},
			},
			wantRescheduleTime: &now,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &RebalancerController{
				Client: fake.NewClientBuilder().WithScheme(gclient.NewSchema()).
					WithObjects(deploy1, binding1, pendingRebalancer, succeedRebalancer, notFoundRebalancer, failedRebalancer).
					WithStatusSubresource(pendingRebalancer, succeedRebalancer, notFoundRebalancer, failedRebalancer).Build(),
			}
			_, err := c.Reconcile(context.TODO(), tt.req)
			// 1. check whether it has error
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// 2. check final WorkloadRebalancer status
			rebalancerGet := &appsv1alpha1.WorkloadRebalancer{}
			if err := c.Client.Get(context.TODO(), tt.req.NamespacedName, rebalancerGet); err != nil {
				t.Errorf("get WorkloadRebalancer failed: %+v", err)
			}
			tt.wantStatus.LastUpdateTime = rebalancerGet.Status.LastUpdateTime
			if !reflect.DeepEqual(rebalancerGet.Status, tt.wantStatus) {
				t.Errorf("update WorkloadRebalancer failed, got: %+v, want: %+v", rebalancerGet.Status, tt.wantStatus)
			}
			// 3. check binding's rescheduleTriggeredAt
			binding1Get := &workv1alpha2.ResourceBinding{}
			if err := c.Client.Get(context.TODO(), client.ObjectKey{Namespace: binding1.Namespace, Name: binding1.Name}, binding1Get); err != nil {
				t.Errorf("get bindding failed: %+v", err)
			}
			if !binding1Get.Spec.RescheduleTriggeredAt.Equal(tt.wantRescheduleTime) {
				t.Errorf("rescheduleTriggeredAt of binding got: %+v, want: %+v", binding1Get.Spec.RescheduleTriggeredAt, tt.wantRescheduleTime)
			}
		})
	}
}

func TestRebalancerController_updateWorkloadRebalancerStatus(t *testing.T) {
	tests := []struct {
		name           string
		oldRebalancer  *appsv1alpha1.WorkloadRebalancer
		newRrebalancer *appsv1alpha1.WorkloadRebalancer
		wantErr        bool
	}{
		{
			name:           "add newStatus to pendingRebalancer",
			oldRebalancer:  pendingRebalancer,
			newRrebalancer: succeedRebalancer,
			wantErr:        false,
		},
		{
			name:           "update status of failedRebalancer to newStatus",
			oldRebalancer:  failedRebalancer,
			newRrebalancer: succeedRebalancer,
			wantErr:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &RebalancerController{
				Client: fake.NewClientBuilder().WithScheme(gclient.NewSchema()).
					WithObjects(tt.oldRebalancer, tt.newRrebalancer).
					WithStatusSubresource(tt.oldRebalancer, tt.newRrebalancer).Build(),
			}
			if err := c.updateWorkloadRebalancerStatus(context.TODO(), tt.oldRebalancer, tt.newRrebalancer); (err != nil) != tt.wantErr {
				t.Errorf("updateWorkloadRebalancerStatus() error = %v, wantErr %v", err, tt.wantErr)
			}
			rebalancerGet := &appsv1alpha1.WorkloadRebalancer{}
			if err := c.Client.Get(context.TODO(), client.ObjectKey{Name: tt.newRrebalancer.Name}, rebalancerGet); err != nil {
				t.Errorf("get WorkloadRebalancer failed: %+v", err)
			}
			if !reflect.DeepEqual(rebalancerGet.Status, tt.newRrebalancer.Status) {
				t.Errorf("update WorkloadRebalancer failed, got: %+v, want: %+v", rebalancerGet.Status, tt.newRrebalancer.Status)
			}
		})
	}
}
