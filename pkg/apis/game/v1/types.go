package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Game struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GameSpec    `json:"spec"`
	Status GamesStatus `json:"status"`
}

type GameSpec struct {
	//game union
	GameID string `json:"gameID"`
	//this game config
	Config string `json:"config"`
	//game images
	Image string `json:"image"`
	//maximum cpu allowed(1000 = 1cpu)
	Cpu uint32 `json:"cpu,omitempty"`
	//maximum memory allowed(1=1Mi)
	Memory uint32 `json:"memory,omitempty"`
	//port
	Port uint32 `json:"port,omitempty"`
	//commonds
	Commonds []string `json:"commonds,omitempty"`
	//replicas
	Replicas uint32 `json:"replicas,omitempty"`
}

type GamesStatus struct {
	//pods
	Pods map[string]*PodStatus `json:"pods,omitempty"`
	//update time
	UpdateAt string `json:"updateAt,omitempty"`
}

type PodStatus struct {
	//name
	Name string `json:"name,omitempty"`
	//host ip
	HostIP string `json:"hostIp,omitempty"`
	//pod ip
	PodIP string `json:"podIp,omitempty"`
	//port
	Port uint32 `json:"port,omitempty"`
	//phase
	Phase corev1.PodPhase `json:"phase,omitempty"`
	//reson
	Events []string `json:"events,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// GameList is a list of Game resources
type GameList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Game `json:"items"`
}
