package field

import (
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var base = time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)

func makeNode(name string, createdSec int) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:              name,
		CreationTimestamp: metav1.NewTime(base.Add(time.Duration(createdSec) * time.Second)),
	}}
}

func makePod(name, node, color string, createdSec int, ready bool) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(base.Add(time.Duration(createdSec) * time.Second)),
		},
		Spec: corev1.PodSpec{NodeName: node},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{Ready: ready}},
		},
	}
	if color != "" {
		p.Annotations = map[string]string{ColorAnnotation: color}
	}
	return p
}

func names(pods []*Pod) map[string]bool {
	m := map[string]bool{}
	for _, p := range pods {
		m[p.Name] = true
	}
	return m
}

func TestNewOrdersColumnsAndStacks(t *testing.T) {
	nodes := []*corev1.Node{makeNode("node-b", 10), makeNode("node-a", 5)}
	pods := []*corev1.Pod{
		makePod("p2", "node-a", "red", 2, true),
		makePod("p1", "node-a", "blue", 1, true),
		makePod("unscheduled", "", "red", 0, true),
	}
	f := New(pods, nodes, AnnotationColor)

	if f.Columns[0].Node.Name != "node-a" || f.Columns[1].Node.Name != "node-b" {
		t.Fatalf("nodes not ordered by creation time: %s, %s", f.Columns[0].Node.Name, f.Columns[1].Node.Name)
	}
	if got := f.Height(0); got != 2 {
		t.Fatalf("expected 2 pods in column 0, got %d", got)
	}
	if f.At(0, 0).Name != "p1" || f.At(0, 1).Name != "p2" {
		t.Fatalf("pods not stacked in creation order: %s, %s", f.At(0, 0).Name, f.At(0, 1).Name)
	}
	if f.Height(1) != 0 {
		t.Fatalf("unscheduled pod must not appear on the field")
	}
}

func TestDropOrderOverridesCreationOrder(t *testing.T) {
	nodes := []*corev1.Node{makeNode("n1", 0)}
	older := makePod("older", "n1", "red", 1, true)
	newer := makePod("newer", "n1", "blue", 2, true)
	// the older pod was dropped later, so it lands on top
	older.Annotations[DropOrderAnnotation] = fmt.Sprint(base.Add(time.Hour).UnixNano())
	newer.Annotations[DropOrderAnnotation] = fmt.Sprint(base.Add(time.Minute).UnixNano())

	f := New([]*corev1.Pod{older, newer}, nodes, AnnotationColor)
	if f.At(0, 0).Name != "newer" || f.At(0, 1).Name != "older" {
		t.Fatalf("drop order not honored: bottom=%s top=%s", f.At(0, 0).Name, f.At(0, 1).Name)
	}
}

func TestErasableVertical(t *testing.T) {
	nodes := []*corev1.Node{makeNode("n1", 0), makeNode("n2", 1)}
	pods := []*corev1.Pod{
		makePod("r1", "n1", "red", 1, true),
		makePod("r2", "n1", "red", 2, true),
		makePod("r3", "n1", "red", 3, true),
		makePod("r4", "n1", "red", 4, true),
		makePod("b1", "n2", "blue", 1, true),
	}
	f := New(pods, nodes, AnnotationColor)
	got := names(f.Erasable(4))
	for _, n := range []string{"r1", "r2", "r3", "r4"} {
		if !got[n] {
			t.Errorf("expected %s to be erasable", n)
		}
	}
	if got["b1"] {
		t.Errorf("b1 must not be erasable")
	}
}

func TestErasableThreeIsNotEnough(t *testing.T) {
	nodes := []*corev1.Node{makeNode("n1", 0)}
	pods := []*corev1.Pod{
		makePod("r1", "n1", "red", 1, true),
		makePod("r2", "n1", "red", 2, true),
		makePod("r3", "n1", "red", 3, true),
	}
	f := New(pods, nodes, AnnotationColor)
	if got := f.Erasable(4); len(got) != 0 {
		t.Fatalf("3 pods must not be erasable with kokeshi=4, got %d", len(got))
	}
	if got := f.Erasable(2); len(got) != 3 {
		t.Fatalf("3 pods must be erasable with kokeshi=2, got %d", len(got))
	}
}

func TestErasableAcrossColumns(t *testing.T) {
	// L-shape: (0,0)(1,0)(1,1)(1,2) all green
	nodes := []*corev1.Node{makeNode("n1", 0), makeNode("n2", 1)}
	pods := []*corev1.Pod{
		makePod("g1", "n1", "green", 1, true),
		makePod("g2", "n2", "green", 1, true),
		makePod("g3", "n2", "green", 2, true),
		makePod("g4", "n2", "green", 3, true),
	}
	f := New(pods, nodes, AnnotationColor)
	if got := f.Erasable(4); len(got) != 4 {
		t.Fatalf("L-shaped group of 4 must be erasable, got %d", len(got))
	}
}

func TestWhiteIsNeverErasableAndDoesNotBridge(t *testing.T) {
	nodes := []*corev1.Node{makeNode("n1", 0)}
	pods := []*corev1.Pod{
		makePod("r1", "n1", "red", 1, true),
		makePod("r2", "n1", "red", 2, true),
		makePod("w1", "n1", "", 3, true), // white splits the column
		makePod("r3", "n1", "red", 4, true),
		makePod("r4", "n1", "red", 5, true),
	}
	f := New(pods, nodes, AnnotationColor)
	if got := f.Erasable(4); len(got) != 0 {
		t.Fatalf("white must not bridge two red groups, got %d erasable", len(got))
	}

	whites := []*corev1.Pod{
		makePod("w1", "n1", "white", 1, true),
		makePod("w2", "n1", "white", 2, true),
		makePod("w3", "n1", "white", 3, true),
		makePod("w4", "n1", "white", 4, true),
	}
	f = New(whites, nodes, AnnotationColor)
	if got := f.Erasable(4); len(got) != 0 {
		t.Fatalf("white pods must never be erasable, got %d", len(got))
	}
}

func TestStable(t *testing.T) {
	nodes := []*corev1.Node{makeNode("n1", 0)}
	ready := makePod("ready", "n1", "red", 1, true)
	notReady := makePod("not-ready", "n1", "red", 2, false)

	if f := New([]*corev1.Pod{ready}, nodes, AnnotationColor); !f.Stable() {
		t.Fatal("field with ready pods must be stable")
	}
	if f := New([]*corev1.Pod{ready, notReady}, nodes, AnnotationColor); f.Stable() {
		t.Fatal("field with a not-ready pod must be unstable")
	}

	terminating := makePod("terminating", "n1", "red", 3, true)
	now := metav1.Now()
	terminating.DeletionTimestamp = &now
	if f := New([]*corev1.Pod{terminating}, nodes, AnnotationColor); f.Stable() {
		t.Fatal("field with a terminating pod must be unstable")
	}
}

func TestHashColorIsDeterministic(t *testing.T) {
	p1 := makePod("a", "n1", "", 1, true)
	p1.Labels = map[string]string{"app": "x"}
	p2 := makePod("b", "n1", "", 2, true)
	p2.Labels = map[string]string{"app": "x"}
	if HashColor(p1) != HashColor(p2) {
		t.Fatal("pods with identical labels must share a color")
	}
}

func TestSelectNodes(t *testing.T) {
	cp := makeNode("cp", 0)
	cp.Labels = map[string]string{ControlPlaneLabel: ""}
	legacy := makeNode("legacy-master", 1)
	legacy.Labels = map[string]string{LegacyControlPlaneLabel: ""}
	w1 := makeNode("w1", 2)
	w1.Labels = map[string]string{"kbkb": "true"}
	w2 := makeNode("w2", 3)
	all := []*corev1.Node{cp, legacy, w1, w2}

	if got := SelectNodes(all, nil, false); len(got) != 4 {
		t.Fatalf("no filter must keep all nodes, got %d", len(got))
	}
	got := SelectNodes(all, nil, true)
	if len(got) != 2 || got[0].Name != "w1" || got[1].Name != "w2" {
		t.Fatalf("control-plane nodes must be excluded, got %v", nodeNames(got))
	}
	got = SelectNodes(all, map[string]string{"kbkb": "true"}, false)
	if len(got) != 1 || got[0].Name != "w1" {
		t.Fatalf("selector must keep only matching nodes, got %v", nodeNames(got))
	}
}

func nodeNames(nodes []*corev1.Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Name
	}
	return out
}
