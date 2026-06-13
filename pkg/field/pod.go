package field

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
)

const (
	// ColorAnnotation is the annotation key that assigns a kbkb color to a Pod.
	ColorAnnotation = "kbkb.k8s.omakenoyouna.net/color"
	// ManagedLabel marks Pods that are spawned and managed by the kbkb controller.
	ManagedLabel = "kbkb.k8s.omakenoyouna.net/managed"
	// OjamaLabel marks garbage Pods sent by an opponent; the scheduler drops
	// them immediately instead of queueing them as an operable pair.
	OjamaLabel = "kbkb.k8s.omakenoyouna.net/ojama"
	// DropOrderAnnotation is stamped by the scheduler right before binding.
	// Pods stack in drop order, so a vertically rotated pair lands the way
	// the player chose; Pods without it stack in creation order.
	DropOrderAnnotation = "kbkb.k8s.omakenoyouna.net/drop-order"

	// ColorWhite is the neutral color: white Pods are never erased and never
	// drag neighbors into a group.
	ColorWhite = "white"
)

// Colors is the palette of erasable colors.
var Colors = []string{"red", "green", "yellow", "blue", "purple"}

// Pod is a Pod placed on the kbkb field, with its resolved color.
type Pod struct {
	*corev1.Pod
	Color string
}

// Stable reports whether the Pod has settled on the field: running, all
// containers ready, and not terminating.
func (p *Pod) Stable() bool {
	if p.DeletionTimestamp != nil {
		return false
	}
	if p.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, cs := range p.Status.ContainerStatuses {
		if !cs.Ready {
			return false
		}
	}
	return true
}

// ColorResolver decides the kbkb color of a Pod.
type ColorResolver func(*corev1.Pod) string

// AnnotationColor resolves the color from the kbkb color annotation.
// Pods without the annotation are white.
func AnnotationColor(p *corev1.Pod) string {
	if c, ok := p.Annotations[ColorAnnotation]; ok && c != "" {
		return c
	}
	return ColorWhite
}

// HashColor resolves the color from a hash of the Pod's labels, so that Pods
// of the same workload share a color. Used by `kubectl kbkb --demo`.
func HashColor(p *corev1.Pod) string {
	b, _ := json.Marshal(p.Labels)
	sum := sha256.Sum256(b)
	return Colors[binary.BigEndian.Uint16(sum[:])%4]
}
