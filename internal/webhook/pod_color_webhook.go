// Package webhook contains the mutating admission webhook that assigns a
// random kbkb color to managed Pods at creation time.
package webhook

import (
	"context"
	"math/rand/v2"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/omakeno/kbkb/v2/pkg/field"
)

// +kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,groups="",resources=pods,verbs=create,versions=v1,name=color.kbkb.k8s.omakenoyouna.net,admissionReviewVersions=v1

// PodColorDefaulter assigns a random color annotation to kbkb-managed Pods
// that do not have one yet. Pods without the managed label are left alone.
type PodColorDefaulter struct {
	// Rand returns a random index in [0, n); overridable for tests.
	Rand func(n int) int
}

// Default implements admission.Defaulter.
func (d *PodColorDefaulter) Default(ctx context.Context, pod *corev1.Pod) error {
	if pod.Labels[field.ManagedLabel] != "true" {
		return nil
	}
	if c, ok := pod.Annotations[field.ColorAnnotation]; ok && c != "" {
		return nil
	}

	pick := d.Rand
	if pick == nil {
		pick = rand.IntN
	}
	color := field.Colors[pick(len(field.Colors))]

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[field.ColorAnnotation] = color
	logf.FromContext(ctx).Info("assigned random color", "pod", pod.GenerateName+pod.Name, "color", color)
	return nil
}

// SetupWebhookWithManager registers the webhook at /mutate--v1-pod.
func (d *PodColorDefaulter) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1.Pod{}).
		WithDefaulter(d).
		Complete()
}
