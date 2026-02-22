/*
Copyright 2026.

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

package v1

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/alxgomz/jolokia-operator/internal/jolokia"
)

// nolint:unused
// log is for logging in this package.
var podlog = logf.Log.WithName("pod-resource")

// SetupPodWebhookWithManager registers the webhook for Pod in the manager.
func SetupPodWebhookWithManager(mgr ctrl.Manager, defaultImage string) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1.Pod{}).
		WithValidator(&PodCustomValidator{}).
		WithDefaulter(&PodCustomDefaulter{DefaultImage: defaultImage}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create,versions=v1,name=mpod-v1.kb.io,admissionReviewVersions=v1,reinvocationPolicy=ifNeeded

// PodCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Pod when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type PodCustomDefaulter struct {
	// DefaultImage is the operator-level default sidecar image,
	// set via --sidecar-image CLI flag.
	DefaultImage string
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Pod.
// It checks for Jolokia annotations and injects the sidecar if present.
func (d *PodCustomDefaulter) Default(_ context.Context, obj *corev1.Pod) error {
	log := podlog.WithValues("pod", obj.GetName(), "namespace", obj.GetNamespace())

	if !jolokia.HasJolokiaAnnotations(obj.Annotations) {
		return nil
	}

	parsed := jolokia.ParseAnnotations(obj.Annotations)

	// Defense-in-depth: reject pods with invalid annotation keys in the
	// defaulting webhook as well (the validating webhook also checks this).
	if len(parsed.InvalidKeys) > 0 {
		log.Info("Rejected Pod with invalid Jolokia annotation keys",
			"invalidKeys", parsed.InvalidKeys)
		return fmt.Errorf("invalid jolokia annotation keys: %s",
			strings.Join(parsed.InvalidKeys, ", "))
	}

	result := jolokia.InjectSidecar(obj, parsed, d.DefaultImage)

	if result.Skipped {
		log.Info("Skipped sidecar injection", "reason", result.SkipReason)
		return nil
	}

	if result.Injected {
		log.Info("Injected Jolokia sidecar", "options", result.OptionsApplied)
	}

	return nil
}

// +kubebuilder:webhook:path=/validate--v1-pod,mutating=false,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create,versions=v1,name=vpod-v1.kb.io,admissionReviewVersions=v1

// PodCustomValidator struct is responsible for validating the Pod resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type PodCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Pod.
// It rejects Pods that have unrecognized jolokia.horoa.net/* annotation keys.
func (v *PodCustomValidator) ValidateCreate(_ context.Context, obj *corev1.Pod) (admission.Warnings, error) {
	log := podlog.WithValues("pod", obj.GetName(), "namespace", obj.GetNamespace())

	if !jolokia.HasJolokiaAnnotations(obj.Annotations) {
		return nil, nil
	}

	parsed := jolokia.ParseAnnotations(obj.Annotations)

	if len(parsed.InvalidKeys) > 0 {
		log.Info("Rejected Pod with invalid Jolokia annotation keys",
			"invalidKeys", parsed.InvalidKeys)
		return nil, fmt.Errorf("invalid jolokia annotation keys: %s",
			strings.Join(parsed.InvalidKeys, ", "))
	}
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Pod.
func (v *PodCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *corev1.Pod) (admission.Warnings, error) {
	podlog.Info("Validation for Pod upon update", "name", newObj.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Pod.
func (v *PodCustomValidator) ValidateDelete(_ context.Context, obj *corev1.Pod) (admission.Warnings, error) {
	podlog.Info("Validation for Pod upon deletion", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
