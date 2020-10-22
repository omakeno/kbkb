package kbkb

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"

	v1 "k8s.io/api/core/v1"
)

type HashedPod struct {
	V1Pod v1.Pod
}

func (kp *HashedPod) Pod() v1.Pod {
	return kp.V1Pod
}

func (kp *HashedPod) IsStable() bool {
	var IsStable bool = true
	for _, containerStatus := range kp.Pod().Status.ContainerStatuses {
		if !containerStatus.Ready {
			IsStable = false
			break
		}
	}
	return IsStable
}

func (kp *HashedPod) Color() string {
	colorMap := map[uint16]string{
		0: "red",
		1: "green",
		2: "yellow",
		3: "blue",
	}

	bytes, _ := json.Marshal(kp.Pod().ObjectMeta.Labels)
	bytes256 := sha256.Sum256(bytes)
	colorInt := binary.BigEndian.Uint16(bytes256[:]) % 4

	return colorMap[colorInt]
}

type HashedPodGenerator struct{}

func (pg *HashedPodGenerator) Generate(p v1.Pod) KbkbPod {
	return &HashedPod{
		V1Pod: p,
	}
}
