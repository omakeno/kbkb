package kbkb

import (
	v1 "k8s.io/api/core/v1"
)

type AnnotatedPod struct {
	V1Pod v1.Pod
}

func (kp *AnnotatedPod) Pod() v1.Pod {
	return kp.V1Pod
}

func (kp *AnnotatedPod) IsStable() bool {
	var IsStable bool = true
	for _, containerStatus := range kp.Pod().Status.ContainerStatuses {
		if !containerStatus.Ready {
			IsStable = false
			break
		}
	}
	return IsStable
}

func (kp *AnnotatedPod) Color() string {
	var color string = "white"
	c, ok := kp.Pod().ObjectMeta.Annotations["kbkb.k8s.omakenoyouna.net/color"]
	if ok {
		color = c
	}
	return color
}

type AnnotatedPodGenerator struct{}

func (pg *AnnotatedPodGenerator) Generate(p v1.Pod) KbkbPod {
	return &AnnotatedPod{
		V1Pod: p,
	}
}
