// Package metrics registers kbkb game metrics on the controller-runtime
// Prometheus registry (exposed on the manager's /metrics endpoint).
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var labels = []string{"namespace", "kbkb"}

var (
	// ChainCurrent is the length of the chain currently in progress.
	ChainCurrent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kbkb_chain_current",
		Help: "Length of the chain currently in progress.",
	}, labels)

	// MaxChain is the longest chain achieved so far.
	MaxChain = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kbkb_max_chain",
		Help: "Longest chain achieved so far. The goal is 19.",
	}, labels)

	// ErasedTotal counts erased Pods.
	ErasedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kbkb_erased_pods_total",
		Help: "Total number of Pods erased.",
	}, labels)

	// SpawnedTotal counts spawned Pods.
	SpawnedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kbkb_spawned_pods_total",
		Help: "Total number of Pods spawned by the spawn controller.",
	}, labels)

	// AllClearTotal counts all-clears (field completely emptied).
	AllClearTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kbkb_all_clear_total",
		Help: "Total number of all-clears.",
	}, labels)

	// OjamaSentTotal counts garbage Pods sent to the opponent namespace.
	OjamaSentTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kbkb_ojama_sent_total",
		Help: "Total number of garbage Pods sent to the opponent namespace.",
	}, labels)

	// GameOver is 1 while the game is over.
	GameOver = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kbkb_game_over",
		Help: "1 while the game is over, 0 otherwise.",
	}, labels)
)

func init() {
	metrics.Registry.MustRegister(
		ChainCurrent, MaxChain, ErasedTotal, SpawnedTotal,
		AllClearTotal, OjamaSentTotal, GameOver,
	)
}
