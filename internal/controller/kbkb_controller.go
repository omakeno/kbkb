package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	k8sv1beta1 "github.com/omakeno/kbkb/v2/api/v1beta1"
	"github.com/omakeno/kbkb/v2/internal/metrics"
	"github.com/omakeno/kbkb/v2/pkg/field"
)

// KbkbReconciler runs the erase side of the game loop: when the field is
// stable, it deletes every group of kokeshi-or-more adjacent same-colored
// Pods, tracks chains, detects all-clears, and sends garbage Pods to the
// opponent in versus mode.
type KbkbReconciler struct {
	client.Client
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=k8s.omakenoyouna.net,resources=kbkbs,verbs=get;list;watch
// +kubebuilder:rbac:groups=k8s.omakenoyouna.net,resources=kbkbs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *KbkbReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var kbkb k8sv1beta1.Kbkb
	if err := r.Get(ctx, req.NamespacedName, &kbkb); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	f, unscheduled, err := r.buildStableField(ctx, &kbkb)
	if err != nil || f == nil {
		return ctrl.Result{}, err
	}

	kokeshi := kbkb.Spec.Kokeshi
	if kokeshi < 2 {
		kokeshi = 4
	}
	erasable := f.Erasable(kokeshi)

	if len(erasable) == 0 {
		return ctrl.Result{}, r.settle(ctx, &kbkb, f, len(unscheduled))
	}
	return ctrl.Result{}, r.erase(ctx, &kbkb, erasable, log)
}

// buildStableField returns the field only when it is stable; an unstable
// field returns nil and waits for the next Pod event.
func (r *KbkbReconciler) buildStableField(ctx context.Context, kbkb *k8sv1beta1.Kbkb) (*field.Field, []corev1.Pod, error) {
	f, unscheduled, err := buildField(ctx, r.Client, kbkb)
	if err != nil {
		return nil, nil, err
	}
	if !f.Stable() {
		return nil, nil, nil
	}
	return f, unscheduled, nil
}

// settle closes the chain in progress, detects all-clears, and resets a
// finished game once the field has been cleaned up.
func (r *KbkbReconciler) settle(ctx context.Context, kbkb *k8sv1beta1.Kbkb, f *field.Field, unscheduled int) error {
	empty := f.MaxHeight() == 0 && unscheduled == 0

	switch kbkb.Status.Phase {
	case k8sv1beta1.PhaseErasing:
		if empty {
			kbkb.Status.AllClears++
			metrics.AllClearTotal.WithLabelValues(kbkb.Namespace, kbkb.Name).Inc()
			r.Recorder.Eventf(kbkb, corev1.EventTypeNormal, "AllClear", "field completely cleared (chain %d)", kbkb.Status.Chain)
		}
	case k8sv1beta1.PhaseGameOver:
		if !empty {
			return nil
		}
		// the field was cleaned up; start a new game
		kbkb.Status.MaxChain = 0
		kbkb.Status.TotalErased = 0
		kbkb.Status.AllClears = 0
		metrics.GameOver.WithLabelValues(kbkb.Namespace, kbkb.Name).Set(0)
	case k8sv1beta1.PhaseIdle:
		return nil
	}

	kbkb.Status.Phase = k8sv1beta1.PhaseIdle
	kbkb.Status.Chain = 0
	metrics.ChainCurrent.WithLabelValues(kbkb.Namespace, kbkb.Name).Set(0)
	return r.Status().Update(ctx, kbkb)
}

// erase deletes the erasable Pods, advances the chain counters and sends
// garbage to the opponent in versus mode.
func (r *KbkbReconciler) erase(ctx context.Context, kbkb *k8sv1beta1.Kbkb, erasable []*field.Pod, log logr.Logger) error {
	if kbkb.Status.Phase == k8sv1beta1.PhaseErasing {
		kbkb.Status.Chain++
	} else {
		kbkb.Status.Chain = 1
	}
	kbkb.Status.Phase = k8sv1beta1.PhaseErasing
	kbkb.Status.MaxChain = max(kbkb.Status.MaxChain, kbkb.Status.Chain)
	kbkb.Status.TotalErased += len(erasable)

	metrics.ChainCurrent.WithLabelValues(kbkb.Namespace, kbkb.Name).Set(float64(kbkb.Status.Chain))
	metrics.MaxChain.WithLabelValues(kbkb.Namespace, kbkb.Name).Set(float64(kbkb.Status.MaxChain))
	metrics.ErasedTotal.WithLabelValues(kbkb.Namespace, kbkb.Name).Add(float64(len(erasable)))

	if err := r.Status().Update(ctx, kbkb); err != nil {
		return err
	}
	r.Recorder.Eventf(kbkb, corev1.EventTypeNormal, "Erase", "erasing %d pods (chain %d)", len(erasable), kbkb.Status.Chain)

	for _, p := range erasable {
		if err := r.Delete(ctx, p.Pod); client.IgnoreNotFound(err) != nil {
			log.Error(err, "failed to delete pod", "pod", p.Name)
		} else {
			log.Info("erased pod", "pod", p.Name, "color", p.Color, "chain", kbkb.Status.Chain)
		}
	}

	return r.sendOjama(ctx, kbkb, len(erasable), log)
}

// sendOjama creates garbage Pods in the opponent namespace in versus mode.
func (r *KbkbReconciler) sendOjama(ctx context.Context, kbkb *k8sv1beta1.Kbkb, erased int, log logr.Logger) error {
	versus := kbkb.Spec.Versus
	if versus == nil || versus.OpponentNamespace == "" || versus.OpponentNamespace == kbkb.Namespace {
		return nil
	}
	rate := versus.GarbageRate
	if rate < 1 {
		rate = 2
	}
	count := erased / rate
	if count == 0 {
		return nil
	}

	image, schedulerName := "", ""
	if kbkb.Spec.Spawn != nil {
		image, schedulerName = kbkb.Spec.Spawn.Image, kbkb.Spec.Spawn.SchedulerName
	}
	for i := 0; i < count; i++ {
		pod := newKbkbPod(versus.OpponentNamespace, image, schedulerName, true)
		if err := r.Create(ctx, pod); err != nil {
			return fmt.Errorf("failed to create ojama pod in %s: %w", versus.OpponentNamespace, err)
		}
	}
	metrics.OjamaSentTotal.WithLabelValues(kbkb.Namespace, kbkb.Name).Add(float64(count))
	r.Recorder.Eventf(kbkb, corev1.EventTypeNormal, "OjamaSent", "sent %d ojama pods to %s", count, versus.OpponentNamespace)
	log.Info("sent ojama pods", "count", count, "opponent", versus.OpponentNamespace)
	return nil
}

// SetupWithManager wires the reconciler: it owns Kbkb objects and re-runs on
// every Pod event in the same namespace.
func (r *KbkbReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("kbkb-erase").
		For(&k8sv1beta1.Kbkb{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(podToKbkbs(mgr.GetClient()))).
		Complete(r)
}
