// The kbkb manager runs the erase controller, the spawn controller and the
// pod-coloring mutating webhook.
package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	k8sv1beta1 "github.com/omakeno/kbkb/v2/api/v1beta1"
	"github.com/omakeno/kbkb/v2/internal/controller"
	kbkbwebhook "github.com/omakeno/kbkb/v2/internal/webhook"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(k8sv1beta1.AddToScheme(scheme))
}

func main() {
	var metricsAddr, probeAddr string
	var enableLeaderElection, enableWebhook bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "address of the metrics endpoint")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "address of the health probe endpoint")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "enable leader election")
	flag.BoolVar(&enableWebhook, "enable-webhook", true, "serve the pod-coloring mutating webhook (requires TLS certs)")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "kbkb.k8s.omakenoyouna.net",
		WebhookServer:          webhook.NewServer(webhook.Options{Port: 9443}),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := (&controller.KbkbReconciler{
		Client:   mgr.GetClient(),
		Recorder: mgr.GetEventRecorderFor("kbkb-erase"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "kbkb-erase")
		os.Exit(1)
	}
	if err := (&controller.SpawnReconciler{
		Client:   mgr.GetClient(),
		Recorder: mgr.GetEventRecorderFor("kbkb-spawn"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "kbkb-spawn")
		os.Exit(1)
	}
	if enableWebhook {
		if err := (&kbkbwebhook.PodColorDefaulter{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "pod-color")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
