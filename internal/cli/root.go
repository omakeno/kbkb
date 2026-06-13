// Package cli implements the kubectl-kbkb plugin command.
package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/cache"

	"github.com/omakeno/kbkb/v2/pkg/field"
	"github.com/omakeno/kbkb/v2/pkg/printer"
)

const example = `
  # view pods in the current namespace
  %[1]s kbkb

  # view pods in a specific namespace
  %[1]s kbkb --namespace your-namespace

  # watch pods
  %[1]s kbkb --watch

  # view with large glyphs (monospaced font required)
  %[1]s kbkb --large

  # color pods by a hash of their labels instead of the annotation
  %[1]s kbkb --demo
`

// Options holds the plugin flags.
type Options struct {
	configFlags         *genericclioptions.ConfigFlags
	watch               bool
	large               bool
	demo                bool
	excludeControlPlane bool
}

// NewCmd builds the kubectl-kbkb root command.
func NewCmd() *cobra.Command {
	o := &Options{configFlags: genericclioptions.NewConfigFlags(true)}
	cmd := &cobra.Command{
		Use:          "kbkb [flags]",
		Short:        "Show pods as kbkb (puyo-puyo) format.",
		Example:      fmt.Sprintf(example, "kubectl"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			return o.Run(c.Context())
		},
	}
	cmd.Flags().BoolVarP(&o.watch, "watch", "w", false, "watch the field")
	cmd.Flags().BoolVarP(&o.large, "large", "L", false, "draw with large glyphs")
	cmd.Flags().BoolVar(&o.demo, "demo", false, "color pods by label hash (for demonstrations)")
	cmd.Flags().BoolVar(&o.excludeControlPlane, "exclude-control-plane", false, "hide control-plane nodes from the field")
	o.configFlags.AddFlags(cmd.Flags())
	return cmd
}

func (o *Options) namespace() string {
	if ns, _, err := o.configFlags.ToRawKubeConfigLoader().Namespace(); err == nil && ns != "" {
		return ns
	}
	return "default"
}

func (o *Options) resolver() field.ColorResolver {
	if o.demo {
		return field.HashColor
	}
	return field.AnnotationColor
}

func (o *Options) charset() printer.CharSet {
	if o.large {
		return printer.Wide()
	}
	return printer.Default()
}

// Run executes the plugin.
func (o *Options) Run(ctx context.Context) error {
	config, err := o.configFlags.ToRESTConfig()
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	if o.watch {
		return o.runWatch(ctx, clientset)
	}
	return o.runOnce(ctx, clientset)
}

func (o *Options) runOnce(ctx context.Context, clientset kubernetes.Interface) error {
	podList, err := clientset.CoreV1().Pods(o.namespace()).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	pods := make([]*corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		if podList.Items[i].Spec.NodeName != "" {
			pods = append(pods, &podList.Items[i])
		}
	}
	nodes := make([]*corev1.Node, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		nodes = append(nodes, &nodeList.Items[i])
	}
	nodes = field.SelectNodes(nodes, nil, o.excludeControlPlane)
	f := field.New(pods, nodes, o.resolver())
	fmt.Fprint(os.Stdout, o.charset().Render(f))
	return nil
}

func (o *Options) runWatch(ctx context.Context, clientset kubernetes.Interface) error {
	factory := informers.NewSharedInformerFactory(clientset, 30*time.Second)
	podInformer := factory.Core().V1().Pods()
	nodeInformer := factory.Core().V1().Nodes()

	ow := &printer.Overwriter{W: os.Stdout}
	redraw := func() {
		podList, err := podInformer.Lister().Pods(o.namespace()).List(labels.Everything())
		if err != nil {
			return
		}
		nodes, err := nodeInformer.Lister().List(labels.Everything())
		if err != nil {
			return
		}
		nodes = field.SelectNodes(nodes, nil, o.excludeControlPlane)
		pods := make([]*corev1.Pod, 0, len(podList))
		for _, p := range podList {
			if p.Spec.NodeName != "" {
				pods = append(pods, p)
			}
		}
		f := field.New(pods, nodes, o.resolver())
		ow.Print(o.charset().Render(f))
	}

	h := cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { redraw() },
		UpdateFunc: func(any, any) { redraw() },
		DeleteFunc: func(any) { redraw() },
	}
	if _, err := podInformer.Informer().AddEventHandler(h); err != nil {
		return err
	}
	if _, err := nodeInformer.Informer().AddEventHandler(h); err != nil {
		return err
	}

	factory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), podInformer.Informer().HasSynced, nodeInformer.Informer().HasSynced)
	redraw()
	<-ctx.Done()
	return nil
}
