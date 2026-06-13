package webhook

import (
	"context"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/omakeno/kbkb/v2/pkg/field"
)

func managedPod() *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:   "p",
		Labels: map[string]string{field.ManagedLabel: "true"},
	}}
}

func TestDefaultAssignsRandomColor(t *testing.T) {
	d := &PodColorDefaulter{Rand: func(n int) int { return 2 }}
	pod := managedPod()
	if err := d.Default(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	if got := pod.Annotations[field.ColorAnnotation]; got != field.Colors[2] {
		t.Fatalf("expected %s, got %q", field.Colors[2], got)
	}
}

func TestDefaultUsesPalette(t *testing.T) {
	d := &PodColorDefaulter{}
	pod := managedPod()
	if err := d.Default(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	if got := pod.Annotations[field.ColorAnnotation]; !slices.Contains(field.Colors, got) {
		t.Fatalf("color %q is not in the palette", got)
	}
}

func TestDefaultSkipsUnmanagedPod(t *testing.T) {
	d := &PodColorDefaulter{}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p"}}
	if err := d.Default(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	if _, ok := pod.Annotations[field.ColorAnnotation]; ok {
		t.Fatal("unmanaged pod must not be colored")
	}
}

func TestDefaultKeepsExistingColor(t *testing.T) {
	d := &PodColorDefaulter{Rand: func(n int) int { return 0 }}
	pod := managedPod()
	pod.Annotations = map[string]string{field.ColorAnnotation: field.ColorWhite}
	if err := d.Default(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	if got := pod.Annotations[field.ColorAnnotation]; got != field.ColorWhite {
		t.Fatalf("existing color must be kept, got %q", got)
	}
}
