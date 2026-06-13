// The kbkb scheduler binds queued Pods two at a time and serves the game web
// UI. Run it in-cluster or locally against a kubeconfig, then open the UI
// (e.g. kubectl port-forward) and play.
package main

import (
	"flag"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	k8sv1beta1 "github.com/omakeno/kbkb/v2/api/v1beta1"
	"github.com/omakeno/kbkb/v2/internal/scheduler"
)

func main() {
	var listenAddr string
	opts := scheduler.Options{}
	flag.StringVar(&opts.SchedulerName, "scheduler-name", "kbkb-scheduler", "schedulerName this scheduler is responsible for")
	flag.StringVar(&opts.Namespace, "namespace", "default", "namespace of the playing field")
	flag.IntVar(&opts.PairSize, "pair", 2, "number of pods operated at a time")
	flag.IntVar(&opts.MaxHeight, "max-height", 12, "column height limit")
	flag.StringVar(&opts.Mode, "mode", "manual", "initial mode: manual (web UI) or auto (random)")
	flag.StringVar(&listenAddr, "listen", ":8765", "address of the web UI / API server")
	zapOpts := zap.Options{Development: true}
	zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))
	log := ctrl.Log.WithName("kbkb-scheduler")

	cfg := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "failed to create clientset")
		os.Exit(1)
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(k8sv1beta1.AddToScheme(scheme))
	kbkbReader, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "failed to create kbkb client; the UI will not show chain stats")
		kbkbReader = nil
	}

	s := scheduler.New(clientset, kbkbReader, opts)
	srv := &scheduler.Server{Scheduler: s}

	mux := http.NewServeMux()
	mux.Handle("/", srv.Handler())
	mux.Handle("/metrics", promhttp.Handler())

	ctx := ctrl.SetupSignalHandler()
	go func() {
		log.Info("serving web UI", "addr", listenAddr)
		server := &http.Server{Addr: listenAddr, Handler: mux}
		go func() {
			<-ctx.Done()
			_ = server.Close()
		}()
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "web server failed")
			os.Exit(1)
		}
	}()

	log.Info("starting scheduler", "namespace", opts.Namespace, "schedulerName", opts.SchedulerName, "mode", opts.Mode)
	if err := s.Run(ctx); err != nil {
		log.Error(err, "scheduler failed")
		os.Exit(1)
	}
}
