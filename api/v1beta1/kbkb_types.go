package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Phase values of a Kbkb game.
const (
	// PhaseIdle: the field is stable and nothing is erasable.
	PhaseIdle = "Idle"
	// PhaseErasing: an erasure happened and the field has not settled yet;
	// further erasures extend the current chain.
	PhaseErasing = "Erasing"
	// PhaseGameOver: a column reached maxHeight; spawning is stopped.
	PhaseGameOver = "GameOver"
)

// KbkbSpec defines the desired state of Kbkb.
type KbkbSpec struct {
	// Kokeshi is the number of adjacent same-colored Pods required to erase.
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:default=4
	// +optional
	Kokeshi int `json:"kokeshi,omitempty"`

	// NodeSelector limits which nodes form the field columns; empty means
	// every node plays.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// ExcludeControlPlane removes control-plane nodes from the field.
	// +optional
	ExcludeControlPlane bool `json:"excludeControlPlane,omitempty"`

	// Spawn configures automatic Pod-pair generation (the "next puyo" feed).
	// +optional
	Spawn *SpawnSpec `json:"spawn,omitempty"`

	// Versus configures the battle mode: chains send garbage (white) Pods to
	// the opponent namespace.
	// +optional
	Versus *VersusSpec `json:"versus,omitempty"`
}

// SpawnSpec configures the spawn controller.
type SpawnSpec struct {
	// Enabled turns automatic spawning on.
	Enabled bool `json:"enabled"`

	// Pair is the number of Pods generated at once.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=2
	// +optional
	Pair int `json:"pair,omitempty"`

	// Image is the container image of spawned Pods.
	// +kubebuilder:default="registry.k8s.io/pause:3.10"
	// +optional
	Image string `json:"image,omitempty"`

	// SchedulerName is set on spawned Pods so that the kbkb scheduler picks
	// them up.
	// +kubebuilder:default="kbkb-scheduler"
	// +optional
	SchedulerName string `json:"schedulerName,omitempty"`

	// MaxHeight is the column height limit; reaching it ends the game.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=12
	// +optional
	MaxHeight int `json:"maxHeight,omitempty"`

	// DisableGameOver keeps spawning even when a column reaches MaxHeight
	// (endless mode). The scheduler still refuses to drop onto full columns.
	// +optional
	DisableGameOver bool `json:"disableGameOver,omitempty"`
}

// VersusSpec configures the battle mode.
type VersusSpec struct {
	// OpponentNamespace receives garbage Pods when this Kbkb erases Pods.
	// +kubebuilder:validation:MinLength=1
	OpponentNamespace string `json:"opponentNamespace"`

	// GarbageRate is how many erased Pods produce one garbage Pod
	// (garbage = erased / garbageRate).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=2
	// +optional
	GarbageRate int `json:"garbageRate,omitempty"`
}

// KbkbStatus defines the observed state of Kbkb.
type KbkbStatus struct {
	// Phase is the current game phase: Idle, Erasing or GameOver.
	// +optional
	Phase string `json:"phase,omitempty"`

	// Chain is the length of the chain currently in progress.
	// +optional
	Chain int `json:"chain,omitempty"`

	// MaxChain is the longest chain achieved so far. The goal is 19.
	// +optional
	MaxChain int `json:"maxChain,omitempty"`

	// TotalErased is the total number of erased Pods.
	// +optional
	TotalErased int `json:"totalErased,omitempty"`

	// AllClears is the number of times the field was completely emptied.
	// +optional
	AllClears int `json:"allClears,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Kokeshi",type=integer,JSONPath=`.spec.kokeshi`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Chain",type=integer,JSONPath=`.status.chain`
// +kubebuilder:printcolumn:name="MaxChain",type=integer,JSONPath=`.status.maxChain`

// Kbkb is the Schema for the kbkbs API.
type Kbkb struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KbkbSpec   `json:"spec,omitempty"`
	Status KbkbStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KbkbList contains a list of Kbkb.
type KbkbList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Kbkb `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Kbkb{}, &KbkbList{})
}
