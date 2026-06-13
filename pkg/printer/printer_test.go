package printer

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/omakeno/kbkb/v2/pkg/field"
)

func testField() *field.Field {
	base := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	node := func(name string, sec int) *corev1.Node {
		return &corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.NewTime(base.Add(time.Duration(sec) * time.Second)),
		}}
	}
	pod := func(name, nodeName, color string, sec int) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				CreationTimestamp: metav1.NewTime(base.Add(time.Duration(sec) * time.Second)),
				Annotations:       map[string]string{field.ColorAnnotation: color},
			},
			Spec: corev1.PodSpec{NodeName: nodeName},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
			},
		}
	}
	return field.New(
		[]*corev1.Pod{pod("r1", "n1", "red", 1), pod("b1", "n1", "blue", 2), pod("g1", "n2", "green", 1)},
		[]*corev1.Node{node("n1", 0), node("n2", 1)},
		field.AnnotationColor,
	)
}

func TestRender(t *testing.T) {
	out := Default().Render(testField())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 2 rows + floor, got %d lines:\n%s", len(lines), out)
	}
	if !strings.HasSuffix(lines[2], "+--+") && lines[2] != "+--+" {
		t.Errorf("floor line malformed: %q", lines[2])
	}
	// top row: blue pod in column 1, blank in column 2
	if !strings.Contains(lines[0], "\033[0;34m@\033[0m") {
		t.Errorf("top row must contain a stable blue pod: %q", lines[0])
	}
	// bottom row: red then green
	if !strings.Contains(lines[1], "\033[0;31m@\033[0m") || !strings.Contains(lines[1], "\033[0;32m@\033[0m") {
		t.Errorf("bottom row must contain red and green pods: %q", lines[1])
	}
}

func TestOverwriterErasesPreviousFrame(t *testing.T) {
	var sb strings.Builder
	ow := &Overwriter{W: &sb}
	ow.Print("a\nb\n")
	ow.Print("c\n")
	out := sb.String()
	if !strings.Contains(out, "\033[2A\033[J") {
		t.Fatalf("second frame must erase the 2-line first frame: %q", out)
	}
}
