package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	k8sv1beta1 "github.com/omakeno/kbkb/v2/api/v1beta1"
	"github.com/omakeno/kbkb/v2/pkg/field"
)

const (
	defaultImage         = "registry.k8s.io/pause:3.10"
	defaultSchedulerName = "kbkb-scheduler"
)

// buildField lists Pods of the Kbkb's namespace and the participating Nodes,
// and assembles the kbkb field. Pods not yet scheduled to a node are returned
// separately.
func buildField(ctx context.Context, c client.Client, kbkb *k8sv1beta1.Kbkb) (*field.Field, []corev1.Pod, error) {
	var podList corev1.PodList
	if err := c.List(ctx, &podList, client.InNamespace(kbkb.Namespace)); err != nil {
		return nil, nil, err
	}
	var nodeList corev1.NodeList
	if err := c.List(ctx, &nodeList); err != nil {
		return nil, nil, err
	}

	pods := make([]*corev1.Pod, 0, len(podList.Items))
	var unscheduled []corev1.Pod
	for i := range podList.Items {
		if podList.Items[i].Spec.NodeName == "" {
			unscheduled = append(unscheduled, podList.Items[i])
			continue
		}
		pods = append(pods, &podList.Items[i])
	}
	nodes := make([]*corev1.Node, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		nodes = append(nodes, &nodeList.Items[i])
	}
	nodes = field.SelectNodes(nodes, kbkb.Spec.NodeSelector, kbkb.Spec.ExcludeControlPlane)

	return field.New(pods, nodes, field.AnnotationColor), unscheduled, nil
}

// podToKbkbs maps a Pod event to every Kbkb in the Pod's namespace, so that
// any Pod change re-triggers the game loop.
func podToKbkbs(c client.Client) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []ctrl.Request {
		var kbkbs k8sv1beta1.KbkbList
		if err := c.List(ctx, &kbkbs, client.InNamespace(obj.GetNamespace())); err != nil {
			return nil
		}
		reqs := make([]ctrl.Request, 0, len(kbkbs.Items))
		for i := range kbkbs.Items {
			reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&kbkbs.Items[i])})
		}
		return reqs
	}
}

// newKbkbPod builds a Pod to be dropped on the field. Without a color
// annotation the mutating webhook assigns a random one; ojama Pods are
// explicitly white and marked so the scheduler drops them immediately.
func newKbkbPod(namespace, image, schedulerName string, ojama bool) *corev1.Pod {
	if image == "" {
		image = defaultImage
	}
	if schedulerName == "" {
		schedulerName = defaultSchedulerName
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "kbkb-",
			Namespace:    namespace,
			Labels:       map[string]string{field.ManagedLabel: "true"},
		},
		Spec: corev1.PodSpec{
			SchedulerName: schedulerName,
			// pause ignores SIGTERM; a long grace period would stall every chain
			TerminationGracePeriodSeconds: ptr.To(int64(1)),
			Containers: []corev1.Container{{
				Name:  "puyo",
				Image: image,
			}},
		},
	}
	if ojama {
		pod.GenerateName = "kbkb-ojama-"
		pod.Labels[field.OjamaLabel] = "true"
		pod.Annotations = map[string]string{field.ColorAnnotation: field.ColorWhite}
	}
	return pod
}
