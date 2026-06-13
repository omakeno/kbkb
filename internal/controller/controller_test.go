package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8sv1beta1 "github.com/omakeno/kbkb/v2/api/v1beta1"
	"github.com/omakeno/kbkb/v2/pkg/field"
)

var testBase = time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := k8sv1beta1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func newClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithStatusSubresource(&k8sv1beta1.Kbkb{}).
		WithObjects(objs...).
		Build()
}

func testNode(name string, sec int) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:              name,
		CreationTimestamp: metav1.NewTime(testBase.Add(time.Duration(sec) * time.Second)),
	}}
}

func testPod(name, ns, node, color string, sec int) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			CreationTimestamp: metav1.NewTime(testBase.Add(time.Duration(sec) * time.Second)),
		},
		Spec: corev1.PodSpec{NodeName: node, Containers: []corev1.Container{{Name: "c", Image: "img"}}},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
		},
	}
	if color != "" {
		p.Annotations = map[string]string{field.ColorAnnotation: color}
	}
	return p
}

func testKbkb(ns string) *k8sv1beta1.Kbkb {
	return &k8sv1beta1.Kbkb{
		ObjectMeta: metav1.ObjectMeta{Name: "kbkb", Namespace: ns},
		Spec:       k8sv1beta1.KbkbSpec{Kokeshi: 4},
	}
}

func reconcileKbkb(t *testing.T, c client.Client, ns string) {
	t.Helper()
	r := &KbkbReconciler{Client: c, Recorder: record.NewFakeRecorder(100)}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Namespace: ns, Name: "kbkb"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func getKbkb(t *testing.T, c client.Client, ns string) *k8sv1beta1.Kbkb {
	t.Helper()
	var k k8sv1beta1.Kbkb
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "kbkb"}, &k); err != nil {
		t.Fatal(err)
	}
	return &k
}

func listPods(t *testing.T, c client.Client, ns string) []corev1.Pod {
	t.Helper()
	var pods corev1.PodList
	if err := c.List(context.Background(), &pods, client.InNamespace(ns)); err != nil {
		t.Fatal(err)
	}
	return pods.Items
}

func TestEraseDeletesFourAdjacentPods(t *testing.T) {
	c := newClient(t,
		testKbkb("default"),
		testNode("n1", 0),
		testPod("r1", "default", "n1", "red", 1),
		testPod("r2", "default", "n1", "red", 2),
		testPod("r3", "default", "n1", "red", 3),
		testPod("r4", "default", "n1", "red", 4),
		testPod("b1", "default", "n1", "blue", 5),
	)
	reconcileKbkb(t, c, "default")

	pods := listPods(t, c, "default")
	if len(pods) != 1 || pods[0].Name != "b1" {
		t.Fatalf("expected only b1 to survive, got %d pods", len(pods))
	}
	k := getKbkb(t, c, "default")
	if k.Status.Phase != k8sv1beta1.PhaseErasing || k.Status.Chain != 1 || k.Status.TotalErased != 4 || k.Status.MaxChain != 1 {
		t.Fatalf("unexpected status: %+v", k.Status)
	}
}

func TestEraseExtendsChain(t *testing.T) {
	kbkb := testKbkb("default")
	kbkb.Status = k8sv1beta1.KbkbStatus{Phase: k8sv1beta1.PhaseErasing, Chain: 3, MaxChain: 3, TotalErased: 12}
	c := newClient(t,
		kbkb,
		testNode("n1", 0),
		testPod("g1", "default", "n1", "green", 1),
		testPod("g2", "default", "n1", "green", 2),
		testPod("g3", "default", "n1", "green", 3),
		testPod("g4", "default", "n1", "green", 4),
	)
	reconcileKbkb(t, c, "default")

	k := getKbkb(t, c, "default")
	if k.Status.Chain != 4 || k.Status.MaxChain != 4 || k.Status.TotalErased != 16 {
		t.Fatalf("chain must extend to 4: %+v", k.Status)
	}
}

func TestEraseSkipsUnstableField(t *testing.T) {
	notReady := testPod("r4", "default", "n1", "red", 4)
	notReady.Status.ContainerStatuses[0].Ready = false
	c := newClient(t,
		testKbkb("default"),
		testNode("n1", 0),
		testPod("r1", "default", "n1", "red", 1),
		testPod("r2", "default", "n1", "red", 2),
		testPod("r3", "default", "n1", "red", 3),
		notReady,
	)
	reconcileKbkb(t, c, "default")

	if pods := listPods(t, c, "default"); len(pods) != 4 {
		t.Fatalf("unstable field must not be erased, got %d pods", len(pods))
	}
}

func TestSettleDetectsAllClear(t *testing.T) {
	kbkb := testKbkb("default")
	kbkb.Status = k8sv1beta1.KbkbStatus{Phase: k8sv1beta1.PhaseErasing, Chain: 2, MaxChain: 2, TotalErased: 8}
	c := newClient(t, kbkb, testNode("n1", 0))
	reconcileKbkb(t, c, "default")

	k := getKbkb(t, c, "default")
	if k.Status.AllClears != 1 || k.Status.Phase != k8sv1beta1.PhaseIdle || k.Status.Chain != 0 {
		t.Fatalf("expected all clear and idle: %+v", k.Status)
	}
	if k.Status.MaxChain != 2 {
		t.Fatalf("maxChain must survive the settle: %+v", k.Status)
	}
}

func TestSettleWithRemainingPodsIsNotAllClear(t *testing.T) {
	kbkb := testKbkb("default")
	kbkb.Status = k8sv1beta1.KbkbStatus{Phase: k8sv1beta1.PhaseErasing, Chain: 1}
	c := newClient(t, kbkb, testNode("n1", 0), testPod("b1", "default", "n1", "blue", 1))
	reconcileKbkb(t, c, "default")

	k := getKbkb(t, c, "default")
	if k.Status.AllClears != 0 || k.Status.Phase != k8sv1beta1.PhaseIdle {
		t.Fatalf("expected idle without all clear: %+v", k.Status)
	}
}

func TestVersusSendsOjama(t *testing.T) {
	kbkb := testKbkb("player1")
	kbkb.Spec.Versus = &k8sv1beta1.VersusSpec{OpponentNamespace: "player2", GarbageRate: 2}
	c := newClient(t,
		kbkb,
		testNode("n1", 0),
		testPod("r1", "player1", "n1", "red", 1),
		testPod("r2", "player1", "n1", "red", 2),
		testPod("r3", "player1", "n1", "red", 3),
		testPod("r4", "player1", "n1", "red", 4),
	)
	reconcileKbkb(t, c, "player1")

	ojama := listPods(t, c, "player2")
	if len(ojama) != 2 {
		t.Fatalf("expected 2 ojama pods (4 erased / rate 2), got %d", len(ojama))
	}
	for _, p := range ojama {
		if p.Labels[field.OjamaLabel] != "true" {
			t.Errorf("ojama pod %s must carry the ojama label", p.Name)
		}
		if p.Annotations[field.ColorAnnotation] != field.ColorWhite {
			t.Errorf("ojama pod %s must be white", p.Name)
		}
	}
}

func TestGameOverResetsAfterCleanup(t *testing.T) {
	kbkb := testKbkb("default")
	kbkb.Status = k8sv1beta1.KbkbStatus{Phase: k8sv1beta1.PhaseGameOver, MaxChain: 5, TotalErased: 40, AllClears: 1}
	c := newClient(t, kbkb, testNode("n1", 0))
	reconcileKbkb(t, c, "default")

	k := getKbkb(t, c, "default")
	if k.Status.Phase != k8sv1beta1.PhaseIdle || k.Status.MaxChain != 0 || k.Status.TotalErased != 0 {
		t.Fatalf("empty field must reset a finished game: %+v", k.Status)
	}
}
