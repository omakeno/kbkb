package scheduler

import (
	"context"
	"strconv"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	"github.com/omakeno/kbkb/v2/pkg/field"
)

var testBase = time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)

func schedNode(name string, sec int) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:              name,
		CreationTimestamp: metav1.NewTime(testBase.Add(time.Duration(sec) * time.Second)),
	}}
}

func schedPod(name, node, color string, sec int, ojama bool) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(testBase.Add(time.Duration(sec) * time.Second)),
			Labels:            map[string]string{field.ManagedLabel: "true"},
			Annotations:       map[string]string{},
		},
		Spec: corev1.PodSpec{NodeName: node, SchedulerName: "kbkb-scheduler"},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
		},
	}
	if node == "" {
		p.Status.Phase = corev1.PodPending
		p.Status.ContainerStatuses = nil
	}
	if color != "" {
		p.Annotations[field.ColorAnnotation] = color
	}
	if ojama {
		p.Labels[field.OjamaLabel] = "true"
	}
	return p
}

// newTestScheduler wires a Scheduler to a fake clientset and hand-fed listers,
// and captures bindings through a reactor.
func newTestScheduler(t *testing.T, opts Options, nodes []*corev1.Node, pods []*corev1.Pod) (*Scheduler, *fake.Clientset, *[]*corev1.Binding) {
	t.Helper()
	objs := make([]runtime.Object, 0, len(nodes)+len(pods))
	for _, n := range nodes {
		objs = append(objs, n)
	}
	for _, p := range pods {
		objs = append(objs, p)
	}
	clientset := fake.NewClientset(objs...)

	binds := &[]*corev1.Binding{}
	clientset.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create, ok := action.(k8stesting.CreateAction)
		if !ok || create.GetSubresource() != "binding" {
			return false, nil, nil
		}
		*binds = append(*binds, create.GetObject().(*corev1.Binding))
		return true, nil, nil
	})

	opts.Now = func() time.Time { return testBase.Add(time.Hour) }
	opts.Rand = func(n int) int { return 0 }
	s := New(clientset, nil, opts)

	podIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	for _, p := range pods {
		if err := podIndexer.Add(p); err != nil {
			t.Fatal(err)
		}
	}
	nodeIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, n := range nodes {
		if err := nodeIndexer.Add(n); err != nil {
			t.Fatal(err)
		}
	}
	s.pods = listerscorev1.NewPodLister(podIndexer)
	s.nodes = listerscorev1.NewNodeLister(nodeIndexer)
	return s, clientset, binds
}

func TestDropBindsPairInOrder(t *testing.T) {
	s, clientset, binds := newTestScheduler(t, Options{MaxHeight: 12},
		[]*corev1.Node{schedNode("n1", 0), schedNode("n2", 1)},
		[]*corev1.Pod{schedPod("p1", "", "red", 1, false), schedPod("p2", "", "blue", 2, false)},
	)

	err := s.Drop(context.Background(), []Placement{{Pod: "p1", Node: "n1"}, {Pod: "p2", Node: "n1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(*binds) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(*binds))
	}
	if (*binds)[0].Name != "p1" || (*binds)[0].Target.Name != "n1" {
		t.Fatalf("first binding wrong: %+v", (*binds)[0])
	}
	if (*binds)[1].Name != "p2" || (*binds)[1].Target.Name != "n1" {
		t.Fatalf("second binding wrong: %+v", (*binds)[1])
	}

	// drop order is stamped and strictly increasing: p1 lands below p2
	o1 := dropOrderOf(t, clientset, "p1")
	o2 := dropOrderOf(t, clientset, "p2")
	if o1 >= o2 {
		t.Fatalf("p1 must stack below p2: %d >= %d", o1, o2)
	}
}

func dropOrderOf(t *testing.T, clientset *fake.Clientset, name string) int64 {
	t.Helper()
	p, err := clientset.CoreV1().Pods("default").Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	v, err := strconv.ParseInt(p.Annotations[field.DropOrderAnnotation], 10, 64)
	if err != nil {
		t.Fatalf("pod %s has no valid drop-order annotation: %v", name, err)
	}
	return v
}

func TestDropRejectsFullColumn(t *testing.T) {
	s, _, binds := newTestScheduler(t, Options{MaxHeight: 1},
		[]*corev1.Node{schedNode("n1", 0)},
		[]*corev1.Pod{
			schedPod("landed", "n1", "red", 1, false),
			schedPod("p1", "", "blue", 2, false),
		},
	)
	err := s.Drop(context.Background(), []Placement{{Pod: "p1", Node: "n1"}})
	if err == nil {
		t.Fatal("dropping onto a full column must fail")
	}
	if len(*binds) != 0 {
		t.Fatal("no binding must happen on a rejected drop")
	}
}

func TestDropRejectsPodNotInQueue(t *testing.T) {
	s, _, _ := newTestScheduler(t, Options{},
		[]*corev1.Node{schedNode("n1", 0)},
		[]*corev1.Pod{schedPod("landed", "n1", "red", 1, false)},
	)
	if err := s.Drop(context.Background(), []Placement{{Pod: "landed", Node: "n1"}}); err == nil {
		t.Fatal("dropping an already-landed pod must fail")
	}
}

func TestPendingSeparatesOjamaAndOrdersQueue(t *testing.T) {
	s, _, _ := newTestScheduler(t, Options{},
		[]*corev1.Node{schedNode("n1", 0)},
		[]*corev1.Pod{
			schedPod("later", "", "red", 5, false),
			schedPod("earlier", "", "blue", 2, false),
			schedPod("garbage", "", "white", 1, true),
			schedPod("landed", "n1", "green", 1, false),
		},
	)
	queue, ojama, err := s.pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 2 || queue[0].Name != "earlier" || queue[1].Name != "later" {
		t.Fatalf("queue must hold pending non-ojama pods in creation order: %v", podNames(queue))
	}
	if len(ojama) != 1 || ojama[0].Name != "garbage" {
		t.Fatalf("ojama must be separated: %v", podNames(ojama))
	}
}

func podNames(pods []*corev1.Pod) []string {
	out := make([]string, len(pods))
	for i, p := range pods {
		out[i] = p.Name
	}
	return out
}

func TestDropOjamaBindsImmediately(t *testing.T) {
	s, _, binds := newTestScheduler(t, Options{},
		[]*corev1.Node{schedNode("n1", 0)},
		[]*corev1.Pod{schedPod("garbage", "", "white", 1, true)},
	)
	if err := s.dropOjama(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*binds) != 1 || (*binds)[0].Name != "garbage" {
		t.Fatalf("ojama must be bound immediately: %v", *binds)
	}
}

func TestAutoDropPlaysThePair(t *testing.T) {
	s, _, binds := newTestScheduler(t, Options{Mode: ModeAuto},
		[]*corev1.Node{schedNode("n1", 0)},
		[]*corev1.Pod{schedPod("p1", "", "red", 1, false), schedPod("p2", "", "blue", 2, false)},
	)
	if err := s.autoDrop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*binds) != 2 {
		t.Fatalf("auto mode must bind the pair, got %d bindings", len(*binds))
	}
}

func TestAutoDropWaitsForStableField(t *testing.T) {
	unstable := schedPod("landed", "n1", "red", 1, false)
	unstable.Status.ContainerStatuses[0].Ready = false
	s, _, binds := newTestScheduler(t, Options{Mode: ModeAuto},
		[]*corev1.Node{schedNode("n1", 0)},
		[]*corev1.Pod{unstable, schedPod("p1", "", "red", 2, false), schedPod("p2", "", "blue", 3, false)},
	)
	if err := s.autoDrop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*binds) != 0 {
		t.Fatal("auto mode must wait for the field to stabilize")
	}
}

func TestSetMode(t *testing.T) {
	s, _, _ := newTestScheduler(t, Options{}, nil, nil)
	if err := s.SetMode("auto"); err != nil || s.Mode() != ModeAuto {
		t.Fatalf("SetMode(auto) failed: %v", err)
	}
	if err := s.SetMode("nonsense"); err == nil {
		t.Fatal("unknown mode must be rejected")
	}
}
