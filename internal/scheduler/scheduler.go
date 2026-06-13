// Package scheduler implements the kbkb custom scheduler: a game server that
// binds queued Pods two at a time, either driven by a player through the web
// UI (manual mode) or randomly (auto mode). Garbage (ojama) Pods are dropped
// immediately without queueing.
package scheduler

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sort"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8sv1beta1 "github.com/omakeno/kbkb/v2/api/v1beta1"
	"github.com/omakeno/kbkb/v2/pkg/field"
)

// Modes of operation.
const (
	ModeManual = "manual"
	ModeAuto   = "auto"
)

// Options configures the scheduler.
type Options struct {
	SchedulerName string
	Namespace     string
	PairSize      int
	MaxHeight     int
	Mode          string
	Resync        time.Duration
	// Rand returns a random index in [0, n); overridable for tests.
	Rand func(n int) int
	// Now returns the current time; overridable for tests.
	Now func() time.Time
}

func (o *Options) defaults() {
	if o.SchedulerName == "" {
		o.SchedulerName = "kbkb-scheduler"
	}
	if o.Namespace == "" {
		o.Namespace = "default"
	}
	if o.PairSize < 1 {
		o.PairSize = 2
	}
	if o.MaxHeight < 1 {
		o.MaxHeight = 12
	}
	if o.Mode == "" {
		o.Mode = ModeManual
	}
	if o.Resync == 0 {
		o.Resync = 30 * time.Second
	}
	if o.Rand == nil {
		o.Rand = rand.IntN
	}
	if o.Now == nil {
		o.Now = time.Now
	}
}

// Placement assigns one queued Pod to one node (column).
type Placement struct {
	Pod  string `json:"pod"`
	Node string `json:"node"`
}

// Scheduler watches Pods and Nodes and binds queued Pods pair by pair.
type Scheduler struct {
	clientset  kubernetes.Interface
	kbkbReader client.Reader // optional, surfaces Kbkb status in the UI
	opts       Options

	pods  listerscorev1.PodLister
	nodes listerscorev1.NodeLister

	mu       sync.Mutex
	mode     string
	inflight map[string]time.Time // bound but not yet observed as scheduled

	notify chan struct{}

	subsMu sync.Mutex
	subs   map[chan struct{}]struct{}
}

// New creates a Scheduler. kbkbReader may be nil.
func New(clientset kubernetes.Interface, kbkbReader client.Reader, opts Options) *Scheduler {
	opts.defaults()
	return &Scheduler{
		clientset:  clientset,
		kbkbReader: kbkbReader,
		opts:       opts,
		mode:       opts.Mode,
		inflight:   map[string]time.Time{},
		notify:     make(chan struct{}, 1),
		subs:       map[chan struct{}]struct{}{},
	}
}

// Run starts the informers and the scheduling loop, blocking until ctx ends.
func (s *Scheduler) Run(ctx context.Context) error {
	factory := informers.NewSharedInformerFactory(s.clientset, s.opts.Resync)
	podInformer := factory.Core().V1().Pods()
	nodeInformer := factory.Core().V1().Nodes()
	s.pods = podInformer.Lister()
	s.nodes = nodeInformer.Lister()

	h := cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { s.poke() },
		UpdateFunc: func(any, any) { s.poke() },
		DeleteFunc: func(any) { s.poke() },
	}
	if _, err := podInformer.Informer().AddEventHandler(h); err != nil {
		return err
	}
	if _, err := nodeInformer.Informer().AddEventHandler(h); err != nil {
		return err
	}

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), podInformer.Informer().HasSynced, nodeInformer.Informer().HasSynced) {
		return fmt.Errorf("failed to sync informer caches")
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.notify:
		case <-ticker.C:
		}
		s.tick(ctx)
	}
}

// poke wakes the scheduling loop and notifies UI subscribers.
func (s *Scheduler) poke() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
	s.broadcast()
}

// tick drops pending ojama Pods immediately, then plays one auto move if in
// auto mode.
func (s *Scheduler) tick(ctx context.Context) {
	s.pruneInflight()
	if err := s.dropOjama(ctx); err != nil {
		fmt.Printf("ojama drop failed: %v\n", err)
	}
	if s.Mode() == ModeAuto {
		if err := s.autoDrop(ctx); err != nil {
			fmt.Printf("auto drop failed: %v\n", err)
		}
	}
}

// Mode returns the current mode.
func (s *Scheduler) Mode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

// SetMode switches between manual and auto.
func (s *Scheduler) SetMode(mode string) error {
	if mode != ModeManual && mode != ModeAuto {
		return fmt.Errorf("unknown mode %q", mode)
	}
	s.mu.Lock()
	s.mode = mode
	s.mu.Unlock()
	s.poke()
	return nil
}

// pending lists Pods waiting for this scheduler, queued in creation order.
func (s *Scheduler) pending() (queue, ojama []*corev1.Pod, err error) {
	all, err := s.pods.Pods(s.opts.Namespace).List(labels.Everything())
	if err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range all {
		if p.Spec.SchedulerName != s.opts.SchedulerName || p.Spec.NodeName != "" || p.DeletionTimestamp != nil {
			continue
		}
		if _, ok := s.inflight[p.Name]; ok {
			continue
		}
		if p.Labels[field.OjamaLabel] == "true" {
			ojama = append(ojama, p)
		} else {
			queue = append(queue, p)
		}
	}
	for _, l := range [][]*corev1.Pod{queue, ojama} {
		sort.Slice(l, func(i, j int) bool {
			if !l[i].CreationTimestamp.Equal(&l[j].CreationTimestamp) {
				return l[i].CreationTimestamp.Before(&l[j].CreationTimestamp)
			}
			return l[i].Name < l[j].Name
		})
	}
	return queue, ojama, nil
}

// buildFieldFor assembles the current field from the informer caches,
// following the Kbkb resource's node filter when available.
func (s *Scheduler) buildFieldFor(kb *k8sv1beta1.Kbkb) (*field.Field, error) {
	nodeList, err := s.nodes.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	if kb != nil {
		nodeList = field.SelectNodes(nodeList, kb.Spec.NodeSelector, kb.Spec.ExcludeControlPlane)
	}
	podList, err := s.pods.Pods(s.opts.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	scheduled := make([]*corev1.Pod, 0, len(podList))
	for _, p := range podList {
		if p.Spec.NodeName != "" {
			scheduled = append(scheduled, p)
		}
	}
	return field.New(scheduled, nodeList, field.AnnotationColor), nil
}

// maxHeightFor is the effective column height limit: the Kbkb resource's
// spawn.maxHeight when set, otherwise the --max-height flag.
func (s *Scheduler) maxHeightFor(kb *k8sv1beta1.Kbkb) int {
	if kb != nil && kb.Spec.Spawn != nil && kb.Spec.Spawn.MaxHeight > 0 {
		return kb.Spec.Spawn.MaxHeight
	}
	return s.opts.MaxHeight
}

// Drop binds the given placements in order; earlier placements land lower.
// Used by the web UI ("drop" button / key) and by auto mode.
func (s *Scheduler) Drop(ctx context.Context, placements []Placement) error {
	if len(placements) == 0 {
		return fmt.Errorf("no placements")
	}
	queue, _, err := s.pending()
	if err != nil {
		return err
	}
	kb := s.kbkb(ctx)
	f, err := s.buildFieldFor(kb)
	if err != nil {
		return err
	}
	maxHeight := s.maxHeightFor(kb)

	queued := map[string]*corev1.Pod{}
	for _, p := range queue {
		queued[p.Name] = p
	}
	heights := map[string]int{}
	for x, col := range f.Columns {
		heights[col.Node.Name] = f.Height(x)
	}

	for _, pl := range placements {
		if _, ok := queued[pl.Pod]; !ok {
			return fmt.Errorf("pod %q is not in the queue", pl.Pod)
		}
		h, ok := heights[pl.Node]
		if !ok {
			return fmt.Errorf("node %q not found", pl.Node)
		}
		if h+1 > maxHeight {
			return fmt.Errorf("column %q is full", pl.Node)
		}
		heights[pl.Node] = h + 1
	}

	for i, pl := range placements {
		if err := s.bind(ctx, queued[pl.Pod], pl.Node, i); err != nil {
			return err
		}
	}
	s.poke()
	return nil
}

// bind stamps the drop order and binds the Pod to the node.
func (s *Scheduler) bind(ctx context.Context, pod *corev1.Pod, node string, seq int) error {
	order := s.opts.Now().UnixNano() + int64(seq)
	patch := fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`, field.DropOrderAnnotation, fmt.Sprint(order))
	if _, err := s.clientset.CoreV1().Pods(pod.Namespace).Patch(ctx, pod.Name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("failed to stamp drop order on %s: %w", pod.Name, err)
	}
	binding := &corev1.Binding{
		ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace, UID: pod.UID},
		Target:     corev1.ObjectReference{Kind: "Node", APIVersion: "v1", Name: node},
	}
	if err := s.clientset.CoreV1().Pods(pod.Namespace).Bind(ctx, binding, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to bind %s to %s: %w", pod.Name, node, err)
	}
	s.mu.Lock()
	s.inflight[pod.Name] = s.opts.Now()
	s.mu.Unlock()
	return nil
}

// dropOjama rains pending garbage Pods onto random columns right away.
func (s *Scheduler) dropOjama(ctx context.Context) error {
	_, ojama, err := s.pending()
	if err != nil || len(ojama) == 0 {
		return err
	}
	kb := s.kbkb(ctx)
	f, err := s.buildFieldFor(kb)
	if err != nil {
		return err
	}
	for _, p := range ojama {
		node := s.pickColumn(f, s.maxHeightFor(kb), true)
		if node == "" {
			return nil
		}
		if err := s.bind(ctx, p, node, 0); err != nil {
			return err
		}
	}
	s.poke()
	return nil
}

// autoDrop plays the next pair onto random columns once the field is stable.
func (s *Scheduler) autoDrop(ctx context.Context) error {
	queue, _, err := s.pending()
	if err != nil || len(queue) < s.opts.PairSize {
		return err
	}
	kb := s.kbkb(ctx)
	f, err := s.buildFieldFor(kb)
	if err != nil {
		return err
	}
	if !f.Stable() {
		return nil
	}
	maxHeight := s.maxHeightFor(kb)
	placements := make([]Placement, 0, s.opts.PairSize)
	heights := map[string]int{}
	for _, p := range queue[:s.opts.PairSize] {
		node := ""
		for range 10 {
			n := s.pickColumn(f, maxHeight, false)
			if n == "" {
				break
			}
			x := s.columnIndex(f, n)
			if f.Height(x)+heights[n] < maxHeight {
				node = n
				break
			}
		}
		if node == "" {
			return nil
		}
		heights[node]++
		placements = append(placements, Placement{Pod: p.Name, Node: node})
	}
	return s.Drop(ctx, placements)
}

// pickColumn picks a random column; unless force, only columns with room.
func (s *Scheduler) pickColumn(f *field.Field, maxHeight int, force bool) string {
	candidates := []string{}
	for x, col := range f.Columns {
		if force || f.Height(x) < maxHeight {
			candidates = append(candidates, col.Node.Name)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[s.opts.Rand(len(candidates))]
}

func (s *Scheduler) columnIndex(f *field.Field, node string) int {
	for x, col := range f.Columns {
		if col.Node.Name == node {
			return x
		}
	}
	return -1
}

// pruneInflight forgets bindings once the cache reflects them (or after 30s).
func (s *Scheduler) pruneInflight() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, t := range s.inflight {
		p, err := s.pods.Pods(s.opts.Namespace).Get(name)
		if err != nil || p.Spec.NodeName != "" || s.opts.Now().Sub(t) > 30*time.Second {
			delete(s.inflight, name)
		}
	}
}

// Subscribe registers a UI listener poked on every cluster change.
func (s *Scheduler) Subscribe() (ch chan struct{}, cancel func()) {
	ch = make(chan struct{}, 1)
	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	return ch, func() {
		s.subsMu.Lock()
		delete(s.subs, ch)
		s.subsMu.Unlock()
	}
}

func (s *Scheduler) broadcast() {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Kbkb returns the first Kbkb object of the namespace, if reachable.
func (s *Scheduler) kbkb(ctx context.Context) *k8sv1beta1.Kbkb {
	if s.kbkbReader == nil {
		return nil
	}
	var list k8sv1beta1.KbkbList
	if err := s.kbkbReader.List(ctx, &list, client.InNamespace(s.opts.Namespace)); err != nil || len(list.Items) == 0 {
		return nil
	}
	return &list.Items[0]
}
