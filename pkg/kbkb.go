package kbkb

import (
	"sort"
	"strings"

	v1 "k8s.io/api/core/v1"
)

type KbkbPod v1.Pod

func (ks *KbkbPod) IsStable() bool {
	var IsStable bool = true
	for _, containerStatus := range ks.Status.ContainerStatuses {
		if !containerStatus.Ready {
			IsStable = false
			break
		}
	}
	return IsStable
}

func (ks *KbkbPod) Color() string {
	var color string = "white"
	c, ok := ks.ObjectMeta.Annotations["kbkbColor"]
	if ok {
		color = c
	}
	return color
}

func (ks *KbkbPod) ColoredString() string {
	var colored string
	switch ks.Color() {
	case "red":
		colored = "\033[0;31m"
	case "green":
		colored = "\033[0;32m"
	case "yellow":
		colored = "\033[0;33m"
	case "blue":
		colored = "\033[0;34m"
	case "purple":
		colored = "\033[0;35m"
	default:
		colored = ""
	}

	var icon string
	if ks.IsStable() {
		icon = "●"
	} else {
		icon = "o"
	}
	return colored + icon + "\033[0m"
}

type KbkbCol struct {
	node  *v1.Node
	kbkbs []*KbkbPod
}

func (kc *KbkbCol) Add(p *KbkbPod) {
	kc.kbkbs = append(kc.kbkbs, p)
}

type KbkbField []*KbkbCol

func BuildKbkbField(p []*v1.Pod, n []*v1.Node) KbkbField {
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

	var kf []*KbkbCol
	kf = make([]*KbkbCol, len(nodes))

	nodenameToIndex := map[string]int{}
	for i, node := range nodes {
		nodenameToIndex[node.Name] = i
		kf[i] = &KbkbCol{
			node:  node,
			kbkbs: []*KbkbPod{},
		}
	}

	for _, pod := range pods {
		kpod := KbkbPod(*pod)
		kf[nodenameToIndex[pod.Spec.NodeName]].Add(&kpod)
	}

	return KbkbField(kf)
}

func (kf KbkbField) PrintAsKbkbOverwrite(p *BashOverwritePrinter) {
	out := strings.Repeat("-", len(kf)+2) + "\n"
	i := 0
	for {
		line := "|"
		empty := true
		for _, kcol := range kf {
			if len(kcol.kbkbs) > i {
				line += kcol.kbkbs[i].ColoredString()
				empty = false
			} else {
				line += " "
			}
		}
		out = line + "|\n" + out
		i++
		if empty {
			break
		}
	}
	p.Print(out)
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

func (kf *KbkbField) GetKbkbPod(x int, y int) *KbkbPod {
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

func (kf *KbkbField) ErasableKbkbPodList(kokeshi int) []*KbkbPod {
	if !kf.IsStable() {
		return nil
	}
	checkedPods := []*KbkbPod{}
	erasablePods := []*KbkbPod{}

	for x, col := range *kf {
		for y, _ := range col.kbkbs {
			var neighborPods []*KbkbPod
			neighborPods, checkedPods = kf.GetNeighbors(x, y, checkedPods)
			if len(neighborPods) >= kokeshi {
				erasablePods = append(erasablePods, neighborPods...)
			}
		}
	}
	return erasablePods
}

func (kf *KbkbField) GetNeighbors(x int, y int, checkedPods []*KbkbPod) (neighborPods, checkedPodsAfter []*KbkbPod) {
	p := kf.GetKbkbPod(x, y)
	neighborPods = []*KbkbPod{p}
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
			var neighborPodsHere []*KbkbPod
			neighborPodsHere, checkedPodsAfter = kf.GetNeighbors(pos[0], pos[1], checkedPodsAfter)
			neighborPods = append(neighborPods, neighborPodsHere...)
		}
	}
	return
}

func contains(pods []*KbkbPod, p *KbkbPod) bool {
	contains := false
	for _, pod := range pods {
		if pod == p {
			contains = true
			break
		}
	}
	return contains
}
