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

package validation

import (
	"fmt"

	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	kubevalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	api "github.com/karmada-io/karmada/pkg/apis/command"
)

const rescheduleNameMaxLength int = 32

// ValidateRescheduleName tests whether the name of Reschedule passed is valid.
// If the name of Reschedule is not valid, a list of error strings is returned. Otherwise, an empty list (or nil) is returned.
// Rules of a valid name of Reschedule:
// - Must be a valid label value as per RFC1123.
//   - An alphanumeric (a-z, and 0-9) string, with a maximum length of 63 characters,
//     with the '-' character allowed anywhere except the first or last character.
//
// - Length must be less than 32 characters.
func ValidateRescheduleName(name string) []string {
	if len(name) == 0 {
		return []string{"must be not empty"}
	}
	if len(name) > rescheduleNameMaxLength {
		return []string{fmt.Sprintf("must be no more than %d characters", rescheduleNameMaxLength)}
	}

	return kubevalidation.IsDNS1123Label(name)
}

// ValidateRescheduleSpec tests if required fields in the RescheduleSpec are set.
func ValidateRescheduleSpec(_ *api.RescheduleSpec, _ *field.Path) field.ErrorList {
	// TODO
	return field.ErrorList{}
}

// ValidateReschedule tests if required fields in the Reschedule are set.
func ValidateReschedule(obj *api.Reschedule) field.ErrorList {
	allErrs := apimachineryvalidation.ValidateObjectMeta(&obj.ObjectMeta, false, func(name string, prefix bool) []string { return ValidateRescheduleName(name) }, field.NewPath("metadata"))
	allErrs = append(allErrs, ValidateRescheduleSpec(&obj.Spec, field.NewPath("spec"))...)
	return allErrs
}
