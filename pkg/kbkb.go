package kbkb

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	v1 "k8s.io/api/core/v1"
)

type KbkbCharSet struct {
	ColorCodeSet map[string]string
	Wall         string
	Floor        string
	LeftCorner   string
	RightCorner  string
	StableIcon   string
	UnstableIcon string
	Blank        string
}

func GetKbkbCharSet() KbkbCharSet {
	return KbkbCharSet{
		ColorCodeSet: map[string]string{
			"red":    "31m",
			"green":  "32m",
			"yellow": "33m",
			"blue":   "34m",
			"purple": "35m",
		},
		Wall:         "|",
		Floor:        "-",
		LeftCorner:   "+",
		RightCorner:  "+",
		StableIcon:   "@",
		UnstableIcon: "o",
		Blank:        " ",
	}
}

func GetKbkbCharSetWide() KbkbCharSet {
	return KbkbCharSet{
		ColorCodeSet: map[string]string{
			"red":    "31m",
			"green":  "32m",
			"yellow": "33m",
			"blue":   "34m",
			"purple": "35m",
		},
		Wall:         "|",
		Floor:        "--",
		LeftCorner:   "+",
		RightCorner:  "+",
		StableIcon:   "●",
		UnstableIcon: "○",
		Blank:        "  ",
	}
}

func (kcs *KbkbCharSet) KbkbPodColor(kp KbkbPod) string {
	colorcode, ok := kcs.ColorCodeSet[kp.Color()]
	if !ok {
		colorcode = "00m"
	}

	var icon string
	if kp.IsStable() {
		icon = kcs.StableIcon
	} else {
		icon = kcs.UnstableIcon
	}
	return "\033[0;" + colorcode + icon + "\033[0m"
}

func (kcs *KbkbCharSet) PrintKbkb(w io.Writer, kf KbkbField) {
	out := kcs.LeftCorner + strings.Repeat(kcs.Floor, len(kf)) + kcs.RightCorner + "\n"
	i := 0
	for {
		line := kcs.Wall
		empty := true
		for _, kcol := range kf {
			if len(kcol.kbkbs) > i {
				line += kcs.KbkbPodColor(kcol.kbkbs[i])
				empty = false
			} else {
				line += kcs.Blank
			}
		}
		out = line + kcs.Wall + "\n" + out
		i++
		if empty {
			break
		}
	}
	fmt.Fprint(w, out)
}

type KbkbPod interface {
	IsStable() bool
	Color() string
	Pod() v1.Pod
}

type KbkbPodGenerator interface {
	Generate(v1.Pod) KbkbPod
}

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

type KbkbCol struct {
	node  v1.Node
	kbkbs []KbkbPod
}

func (kc *KbkbCol) Add(p KbkbPod) {
	kc.kbkbs = append(kc.kbkbs, p)
}

type KbkbField []KbkbCol

func BuildKbkbFieldFromList(pl *v1.PodList, nl *v1.NodeList, pg KbkbPodGenerator) KbkbField {
	p := []*v1.Pod{}
	n := []*v1.Node{}

	for i, l := 0, len(pl.Items); i < l; i++ {
		p = append(p, &pl.Items[i])
	}

	for i, l := 0, len(nl.Items); i < l; i++ {
		n = append(n, &nl.Items[i])
	}

	return BuildKbkbField(p, n, pg)
}

func BuildKbkbField(p []*v1.Pod, n []*v1.Node, pg KbkbPodGenerator) KbkbField {
	var nodes []*v1.Node = make([]*v1.Node, len(n))
	copy(nodes, n)
	var pods []*v1.Pod = make([]*v1.Pod, len(p))
	copy(pods, p)

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].CreationTimestamp.UnixNano() == nodes[j].CreationTimestamp.UnixNano() {
			return nodes[i].Name < nodes[j].Name
		} else {
			return nodes[i].CreationTimestamp.UnixNano() < nodes[j].CreationTimestamp.UnixNano()
		}
	})
	sort.Slice(pods, func(i, j int) bool {
		if pods[i].CreationTimestamp.UnixNano() == pods[j].CreationTimestamp.UnixNano() {
			return pods[i].Name < pods[j].Name
		} else {
			return pods[i].CreationTimestamp.UnixNano() < pods[j].CreationTimestamp.UnixNano()
		}
	})

	var kf []KbkbCol
	kf = make([]KbkbCol, len(nodes))

	nodenameToIndex := map[string]int{}
	for i, node := range nodes {
		nodenameToIndex[node.Name] = i
		kf[i] = KbkbCol{
			node:  *node,
			kbkbs: []KbkbPod{},
		}
	}

	for _, pod := range pods {
		kpod := pg.Generate(*pod)
		kf[nodenameToIndex[pod.Spec.NodeName]].Add(kpod)
	}

	return KbkbField(kf)
}

func (kf *KbkbField) IsStable() bool {
	isStable := true
	for _, kcol := range *kf {
		for _, kpod := range kcol.kbkbs {
			if !kpod.IsStable() {
				isStable = false
				break
			}
		}
		if !isStable {
			break
		}
	}
	return isStable
}

func (kf *KbkbField) GetKbkbPod(x int, y int) KbkbPod {
	if x < 0 || y < 0 {
		return nil
	}
	if (len(*kf)) > x {
		if len((*kf)[x].kbkbs) > y {
			return (*kf)[x].kbkbs[y]
		}
	}
	return nil
}

func (kf *KbkbField) ErasableKbkbPodList(kokeshi int) []KbkbPod {
	checkedPods := []KbkbPod{}
	erasablePods := []KbkbPod{}

	for x, col := range *kf {
		for y, _ := range col.kbkbs {
			var neighborPods []KbkbPod
			neighborPods, checkedPods = kf.getNeighbors(x, y, checkedPods)
			if len(neighborPods) >= kokeshi {
				erasablePods = append(erasablePods, neighborPods...)
			}
		}
	}
	return erasablePods
}

func (kf *KbkbField) getNeighbors(x int, y int, checkedPods []KbkbPod) (neighborPods, checkedPodsAfter []KbkbPod) {
	p := kf.GetKbkbPod(x, y)
	neighborPods = []KbkbPod{p}
	if contains(checkedPods, p) {
		checkedPodsAfter = checkedPods
		return
	}
	checkedPodsAfter = append(checkedPods, p)

	if p.Color() == "white" {
		return
	}

	neighborPos := [][]int{
		{x + 1, y},
		{x - 1, y},
		{x, y + 1},
		{x, y - 1},
	}
	for _, pos := range neighborPos {
		if np := kf.GetKbkbPod(pos[0], pos[1]); np != nil && !contains(checkedPodsAfter, np) && np.Color() == p.Color() {
			var neighborPodsHere []KbkbPod
			neighborPodsHere, checkedPodsAfter = kf.getNeighbors(pos[0], pos[1], checkedPodsAfter)
			neighborPods = append(neighborPods, neighborPodsHere...)
		}
	}
	return
}

func contains(pods []KbkbPod, p KbkbPod) bool {
	contains := false
	for _, pod := range pods {
		if pod == p {
			contains = true
			break
		}
	}
	return contains
}
