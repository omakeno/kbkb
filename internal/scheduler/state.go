package scheduler

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/omakeno/kbkb/v2/pkg/field"
)

// PodState is one Pod as shown in the web UI.
type PodState struct {
	Name   string `json:"name"`
	Color  string `json:"color"`
	Stable bool   `json:"stable"`
	Ojama  bool   `json:"ojama"`
	Phase  string `json:"phase"`
	Node   string `json:"node,omitempty"`
	Ready  bool   `json:"ready"`
	// Manifest is a ~10-line YAML excerpt of the Pod, shown on hover.
	Manifest string `json:"manifest"`
}

// manifestSnippet renders a compact kubectl-style excerpt of the Pod:
// metadata is trimmed to name/namespace, spec to the bits that matter on the
// field (container image, nodeName, schedulerName).
func manifestSnippet(p *corev1.Pod) string {
	var b strings.Builder
	b.WriteString("apiVersion: v1\nkind: Pod\nmetadata:\n")
	fmt.Fprintf(&b, "  name: %s\n  namespace: %s\n", p.Name, p.Namespace)
	b.WriteString("spec:\n  containers:\n")
	for _, c := range p.Spec.Containers {
		fmt.Fprintf(&b, "  - name: %s\n    image: %s\n", c.Name, c.Image)
	}
	if p.Spec.NodeName != "" {
		fmt.Fprintf(&b, "  nodeName: %s\n", p.Spec.NodeName)
	}
	if p.Spec.SchedulerName != "" {
		fmt.Fprintf(&b, "  schedulerName: %s\n", p.Spec.SchedulerName)
	}
	phase := string(p.Status.Phase)
	if p.DeletionTimestamp != nil {
		phase = "Terminating"
	}
	fmt.Fprintf(&b, "status:\n  phase: %s", phase)
	return b.String()
}

// podState flattens a Pod for the UI; a deleted-but-running Pod shows as
// Terminating, the way kubectl does.
func podState(p *corev1.Pod, color string, stable, ojama bool) PodState {
	phase := string(p.Status.Phase)
	if p.DeletionTimestamp != nil {
		phase = "Terminating"
	}
	ready := len(p.Status.ContainerStatuses) > 0
	for _, cs := range p.Status.ContainerStatuses {
		if !cs.Ready {
			ready = false
			break
		}
	}
	return PodState{
		Name:     p.Name,
		Color:    color,
		Stable:   stable,
		Ojama:    ojama,
		Phase:    phase,
		Node:     p.Spec.NodeName,
		Ready:    ready,
		Manifest: manifestSnippet(p),
	}
}

// ColumnState is one node column as shown in the web UI.
type ColumnState struct {
	Node string     `json:"node"`
	Pods []PodState `json:"pods"`
}

// State is the full game state pushed to the web UI.
type State struct {
	Columns   []ColumnState `json:"columns"`
	Queue     []PodState    `json:"queue"`
	Mode      string        `json:"mode"`
	PairSize  int           `json:"pairSize"`
	MaxHeight int           `json:"maxHeight"`
	Namespace string        `json:"namespace"`
	Stable    bool          `json:"stable"`

	// mirrored from the Kbkb resource when available
	Phase       string `json:"phase,omitempty"`
	Chain       int    `json:"chain"`
	MaxChain    int    `json:"maxChain"`
	TotalErased int    `json:"totalErased"`
	AllClears   int    `json:"allClears"`
}

// State assembles the current game state.
func (s *Scheduler) State(ctx context.Context) (*State, error) {
	kb := s.kbkb(ctx)
	f, err := s.buildFieldFor(kb)
	if err != nil {
		return nil, err
	}
	queue, ojama, err := s.pending()
	if err != nil {
		return nil, err
	}

	st := &State{
		Columns:   make([]ColumnState, 0, len(f.Columns)),
		Queue:     []PodState{},
		Mode:      s.Mode(),
		PairSize:  s.opts.PairSize,
		MaxHeight: s.maxHeightFor(kb),
		Namespace: s.opts.Namespace,
		Stable:    f.Stable(),
	}
	for _, col := range f.Columns {
		cs := ColumnState{Node: col.Node.Name, Pods: []PodState{}}
		for _, p := range col.Pods {
			cs.Pods = append(cs.Pods, podState(p.Pod, p.Color, p.Stable(), p.Labels[field.OjamaLabel] == "true"))
		}
		st.Columns = append(st.Columns, cs)
	}
	for _, p := range queue {
		st.Queue = append(st.Queue, podState(p, field.AnnotationColor(p), false, false))
	}
	for _, p := range ojama {
		st.Queue = append(st.Queue, podState(p, field.ColorWhite, false, true))
	}

	if kb != nil {
		st.Phase = kb.Status.Phase
		st.Chain = kb.Status.Chain
		st.MaxChain = kb.Status.MaxChain
		st.TotalErased = kb.Status.TotalErased
		st.AllClears = kb.Status.AllClears
	}
	return st, nil
}
