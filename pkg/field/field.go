package field

import (
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

// Column is one node of the cluster, holding its Pods bottom-up in creation
// order.
type Column struct {
	Node *corev1.Node
	Pods []*Pod
}

// Field is the kbkb playing field: nodes are columns, Pods stack up inside
// each column in creation order.
type Field struct {
	Columns []Column
}

// New builds a Field from Pods and Nodes. Nodes are ordered by creation time
// (then name), and so are the Pods within each column. Pods that are not
// scheduled to any known node are ignored.
func New(pods []*corev1.Pod, nodes []*corev1.Node, resolve ColorResolver) *Field {
	sortedNodes := make([]*corev1.Node, len(nodes))
	copy(sortedNodes, nodes)
	sort.Slice(sortedNodes, func(i, j int) bool {
		if !sortedNodes[i].CreationTimestamp.Equal(&sortedNodes[j].CreationTimestamp) {
			return sortedNodes[i].CreationTimestamp.Before(&sortedNodes[j].CreationTimestamp)
		}
		return sortedNodes[i].Name < sortedNodes[j].Name
	})

	sortedPods := make([]*corev1.Pod, len(pods))
	copy(sortedPods, pods)
	sort.Slice(sortedPods, func(i, j int) bool {
		ki, kj := stackOrder(sortedPods[i]), stackOrder(sortedPods[j])
		if ki != kj {
			return ki < kj
		}
		return sortedPods[i].Name < sortedPods[j].Name
	})

	f := &Field{Columns: make([]Column, len(sortedNodes))}
	colIndex := make(map[string]int, len(sortedNodes))
	for i, n := range sortedNodes {
		f.Columns[i] = Column{Node: n}
		colIndex[n.Name] = i
	}

	for _, p := range sortedPods {
		i, ok := colIndex[p.Spec.NodeName]
		if !ok {
			continue
		}
		f.Columns[i].Pods = append(f.Columns[i].Pods, &Pod{Pod: p, Color: resolve(p)})
	}
	return f
}

// Control-plane node role labels (current and legacy).
const (
	ControlPlaneLabel       = "node-role.kubernetes.io/control-plane"
	LegacyControlPlaneLabel = "node-role.kubernetes.io/master"
)

// SelectNodes filters the nodes that form the field columns: nodes must match
// every label in selector (empty selector matches all), and control-plane
// nodes are removed when excludeControlPlane is set.
func SelectNodes(nodes []*corev1.Node, selector map[string]string, excludeControlPlane bool) []*corev1.Node {
	out := make([]*corev1.Node, 0, len(nodes))
	for _, n := range nodes {
		if excludeControlPlane {
			if _, ok := n.Labels[ControlPlaneLabel]; ok {
				continue
			}
			if _, ok := n.Labels[LegacyControlPlaneLabel]; ok {
				continue
			}
		}
		matches := true
		for k, v := range selector {
			if n.Labels[k] != v {
				matches = false
				break
			}
		}
		if matches {
			out = append(out, n)
		}
	}
	return out
}

// stackOrder is the key Pods stack by: the scheduler's drop order when
// present, otherwise the creation time.
func stackOrder(p *corev1.Pod) int64 {
	if v, ok := p.Annotations[DropOrderAnnotation]; ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return p.CreationTimestamp.UnixNano()
}

// At returns the Pod at column x, height y, or nil if out of range.
func (f *Field) At(x, y int) *Pod {
	if x < 0 || x >= len(f.Columns) || y < 0 || y >= len(f.Columns[x].Pods) {
		return nil
	}
	return f.Columns[x].Pods[y]
}

// Stable reports whether every Pod on the field has settled.
func (f *Field) Stable() bool {
	for _, col := range f.Columns {
		for _, p := range col.Pods {
			if !p.Stable() {
				return false
			}
		}
	}
	return true
}

// Height returns the number of Pods stacked in column x.
func (f *Field) Height(x int) int {
	if x < 0 || x >= len(f.Columns) {
		return 0
	}
	return len(f.Columns[x].Pods)
}

// MaxHeight returns the height of the tallest column.
func (f *Field) MaxHeight() int {
	max := 0
	for x := range f.Columns {
		if h := f.Height(x); h > max {
			max = h
		}
	}
	return max
}

// Erasable returns every Pod that belongs to a group of `threshold` or more
// same-colored adjacent Pods. White Pods never form groups.
func (f *Field) Erasable(threshold int) []*Pod {
	visited := make([][]bool, len(f.Columns))
	for x := range f.Columns {
		visited[x] = make([]bool, len(f.Columns[x].Pods))
	}

	var erasable []*Pod
	for x := range f.Columns {
		for y := range f.Columns[x].Pods {
			if visited[x][y] {
				continue
			}
			group := f.group(x, y, visited)
			if len(group) >= threshold {
				erasable = append(erasable, group...)
			}
		}
	}
	return erasable
}

// group collects the connected same-colored group containing (x, y) with an
// iterative depth-first search, marking every visited cell. White cells form
// no group.
func (f *Field) group(x, y int, visited [][]bool) []*Pod {
	start := f.At(x, y)
	visited[x][y] = true
	if start.Color == ColorWhite {
		return nil
	}

	group := []*Pod{start}
	stack := [][2]int{{x, y}}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := cur[0]+d[0], cur[1]+d[1]
			np := f.At(nx, ny)
			if np == nil || visited[nx][ny] || np.Color != start.Color {
				continue
			}
			visited[nx][ny] = true
			group = append(group, np)
			stack = append(stack, [2]int{nx, ny})
		}
	}
	return group
}
