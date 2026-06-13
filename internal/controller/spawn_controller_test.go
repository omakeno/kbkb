package controller

import (
	"context"
	"testing"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8sv1beta1 "github.com/omakeno/kbkb/v2/api/v1beta1"
	"github.com/omakeno/kbkb/v2/pkg/field"
)

func reconcileSpawn(t *testing.T, c client.Client, ns string) {
	t.Helper()
	r := &SpawnReconciler{Client: c, Recorder: record.NewFakeRecorder(100)}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Namespace: ns, Name: "kbkb"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func spawnKbkb(ns string) *k8sv1beta1.Kbkb {
	k := testKbkb(ns)
	k.Spec.Spawn = &k8sv1beta1.SpawnSpec{
		Enabled:       true,
		Pair:          2,
		Image:         "registry.k8s.io/pause:3.10",
		SchedulerName: "kbkb-scheduler",
		MaxHeight:     3,
	}
	return k
}

func TestSpawnCreatesAPairWhenStable(t *testing.T) {
	c := newClient(t,
		spawnKbkb("default"),
		testNode("n1", 0),
		testPod("b1", "default", "n1", "blue", 1),
	)
	reconcileSpawn(t, c, "default")

	pods := listPods(t, c, "default")
	if len(pods) != 3 {
		t.Fatalf("expected 1 existing + 2 spawned pods, got %d", len(pods))
	}
	spawned := 0
	for _, p := range pods {
		if p.Labels[field.ManagedLabel] != "true" {
			continue
		}
		spawned++
		if p.Spec.SchedulerName != "kbkb-scheduler" {
			t.Errorf("spawned pod %s must use the kbkb scheduler, got %q", p.Name, p.Spec.SchedulerName)
		}
		if _, ok := p.Annotations[field.ColorAnnotation]; ok {
			t.Errorf("spawned pod %s must leave coloring to the webhook", p.Name)
		}
		if p.Spec.NodeName != "" {
			t.Errorf("spawned pod %s must be unscheduled", p.Name)
		}
	}
	if spawned != 2 {
		t.Fatalf("expected exactly 2 spawned pods, got %d", spawned)
	}
}

func TestSpawnWaitsForPendingPods(t *testing.T) {
	pending := testPod("pending", "default", "", "", 1)
	pending.Labels = map[string]string{field.ManagedLabel: "true"}
	c := newClient(t, spawnKbkb("default"), testNode("n1", 0), pending)
	reconcileSpawn(t, c, "default")

	if pods := listPods(t, c, "default"); len(pods) != 1 {
		t.Fatalf("must not spawn while a pod waits for a node, got %d pods", len(pods))
	}
}

func TestSpawnWaitsForErasableField(t *testing.T) {
	c := newClient(t,
		spawnKbkb("default"),
		testNode("n1", 0), testNode("n2", 1),
		testPod("r1", "default", "n1", "red", 1),
		testPod("r2", "default", "n1", "red", 2),
		testPod("r3", "default", "n2", "red", 1),
		testPod("r4", "default", "n2", "red", 2),
	)
	reconcileSpawn(t, c, "default")

	if pods := listPods(t, c, "default"); len(pods) != 4 {
		t.Fatalf("must not spawn while pods are erasable, got %d pods", len(pods))
	}
}

func TestSpawnGameOverAtMaxHeight(t *testing.T) {
	c := newClient(t,
		spawnKbkb("default"), // maxHeight: 3
		testNode("n1", 0),
		testPod("b1", "default", "n1", "blue", 1),
		testPod("g1", "default", "n1", "green", 2),
		testPod("p1", "default", "n1", "purple", 3),
	)
	reconcileSpawn(t, c, "default")

	if pods := listPods(t, c, "default"); len(pods) != 3 {
		t.Fatalf("must not spawn past maxHeight, got %d pods", len(pods))
	}
	k := getKbkb(t, c, "default")
	if k.Status.Phase != k8sv1beta1.PhaseGameOver {
		t.Fatalf("expected GameOver, got %q", k.Status.Phase)
	}

	// once over, the game stays frozen
	reconcileSpawn(t, c, "default")
	if pods := listPods(t, c, "default"); len(pods) != 3 {
		t.Fatal("game over must stop spawning")
	}
}

func TestSpawnDisabled(t *testing.T) {
	k := spawnKbkb("default")
	k.Spec.Spawn.Enabled = false
	c := newClient(t, k, testNode("n1", 0))
	reconcileSpawn(t, c, "default")

	if pods := listPods(t, c, "default"); len(pods) != 0 {
		t.Fatalf("disabled spawn must not create pods, got %d", len(pods))
	}
}

func TestSpawnEndlessModeSkipsGameOver(t *testing.T) {
	k := spawnKbkb("default") // maxHeight: 3
	k.Spec.Spawn.DisableGameOver = true
	c := newClient(t, k,
		testNode("n1", 0),
		testPod("b1", "default", "n1", "blue", 1),
		testPod("g1", "default", "n1", "green", 2),
		testPod("p1", "default", "n1", "purple", 3),
	)
	reconcileSpawn(t, c, "default")

	if got := getKbkb(t, c, "default").Status.Phase; got == k8sv1beta1.PhaseGameOver {
		t.Fatal("endless mode must not trigger game over")
	}
	if pods := listPods(t, c, "default"); len(pods) != 5 {
		t.Fatalf("endless mode must keep spawning, got %d pods", len(pods))
	}
}

func TestExcludedNodesAreOffTheField(t *testing.T) {
	cpNode := testNode("cp", 0)
	cpNode.Labels = map[string]string{field.ControlPlaneLabel: ""}
	kbkb := testKbkb("default")
	kbkb.Spec.ExcludeControlPlane = true
	c := newClient(t,
		kbkb,
		cpNode,
		testNode("n1", 1),
		testPod("r1", "default", "cp", "red", 1),
		testPod("r2", "default", "cp", "red", 2),
		testPod("r3", "default", "cp", "red", 3),
		testPod("r4", "default", "cp", "red", 4),
	)
	reconcileKbkb(t, c, "default")

	if pods := listPods(t, c, "default"); len(pods) != 4 {
		t.Fatalf("pods on an excluded node must never be erased, got %d", len(pods))
	}
}
