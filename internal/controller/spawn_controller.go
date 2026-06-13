package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	k8sv1beta1 "github.com/omakeno/kbkb/v2/api/v1beta1"
	"github.com/omakeno/kbkb/v2/internal/metrics"
)

// SpawnReconciler runs the feed side of the game loop: once every Pod on the
// field is Running and Ready, nothing is erasable, and no spawned Pod is
// still waiting for a node, it generates the next operable pair. When a
// column reaches maxHeight the game is over and spawning stops.
type SpawnReconciler struct {
	client.Client
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=k8s.omakenoyouna.net,resources=kbkbs,verbs=get;list;watch
// +kubebuilder:rbac:groups=k8s.omakenoyouna.net,resources=kbkbs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *SpawnReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var kbkb k8sv1beta1.Kbkb
	if err := r.Get(ctx, req.NamespacedName, &kbkb); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	spawn := kbkb.Spec.Spawn
	if spawn == nil || !spawn.Enabled || kbkb.Status.Phase == k8sv1beta1.PhaseGameOver {
		return ctrl.Result{}, nil
	}

	f, unscheduled, err := buildField(ctx, r.Client, &kbkb)
	if err != nil {
		return ctrl.Result{}, err
	}

	// wait until the previous pair (or ojama) has landed and settled
	if len(unscheduled) > 0 || !f.Stable() {
		return ctrl.Result{}, nil
	}

	kokeshi := kbkb.Spec.Kokeshi
	if kokeshi < 2 {
		kokeshi = 4
	}
	// the erase controller acts first
	if len(f.Erasable(kokeshi)) > 0 {
		return ctrl.Result{}, nil
	}

	maxHeight := spawn.MaxHeight
	if maxHeight < 1 {
		maxHeight = 12
	}
	if !spawn.DisableGameOver && f.MaxHeight() >= maxHeight {
		kbkb.Status.Phase = k8sv1beta1.PhaseGameOver
		metrics.GameOver.WithLabelValues(kbkb.Namespace, kbkb.Name).Set(1)
		r.Recorder.Eventf(&kbkb, corev1.EventTypeWarning, "GameOver", "a column reached maxHeight (%d); max chain was %d", maxHeight, kbkb.Status.MaxChain)
		log.Info("game over", "maxHeight", maxHeight, "maxChain", kbkb.Status.MaxChain)
		return ctrl.Result{}, r.Status().Update(ctx, &kbkb)
	}

	pair := spawn.Pair
	if pair < 1 {
		pair = 2
	}
	for i := 0; i < pair; i++ {
		pod := newKbkbPod(kbkb.Namespace, spawn.Image, spawn.SchedulerName, false)
		if err := r.Create(ctx, pod); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("spawned pod", "pod", pod.Name)
	}
	metrics.SpawnedTotal.WithLabelValues(kbkb.Namespace, kbkb.Name).Add(float64(pair))
	r.Recorder.Eventf(&kbkb, corev1.EventTypeNormal, "Spawn", "spawned %d pods", pair)
	return ctrl.Result{}, nil
}

// SetupWithManager wires the reconciler to Kbkb and Pod events.
func (r *SpawnReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("kbkb-spawn").
		For(&k8sv1beta1.Kbkb{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(podToKbkbs(mgr.GetClient()))).
		Complete(r)
}
