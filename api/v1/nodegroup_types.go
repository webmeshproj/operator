/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeGroupSpec is the specification for a group of nodes.
type NodeGroupSpec struct {
	// Image is the image to use for the node.
	// +kubebuilder:default:="ghcr.io/webmeshproj/node:latest"
	// +optional
	Image string `json:"image,omitempty"`

	// ImagePullPolicy is the image pull policy to use for the node.
	// +kubebuilder:default:="IfNotPresent"
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// ImagePullSecrets is the list of image pull secrets to use for the
	// node.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Replicas is the number of replicas to run for this group.
	// +kubebuilder:default:=1
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// PodAnnotations is the annotations to use for the node containers in
	// this group.
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`

	// HostNetwork is whether to use host networking for the node
	// containers in this group.
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty"`

	// NodeSelector is the node selector to use for the node containers in
	// this group.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Affininity is the affinity to use for the node containers in this
	// group.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Tolerations is the tolerations to use for the node containers in
	// this group.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// PreemptionPolicy is the preemption policy to use for the node
	// containers in this group.
	// +kubebuilder:default:="PreemptLowerPriority"
	// +optional
	PreemptionPolicy corev1.PreemptionPolicy `json:"preemptionPolicy,omitempty"`

	// TopologySpreadConstraints is the topology spread constraints to use
	// for the node containers in this group.
	// +optional
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

	// ResourceClaims is the resource claims to use for the node containers
	// in this group.
	// +optional
	ResourceClaims []corev1.PodResourceClaim `json:"resourceClaims,omitempty"`

	// AdditionalVolumes is the additional volumes to use for the node
	// containers in this group.
	// +optional
	AdditionalVolumes []corev1.Volume `json:"additionalVolumes,omitempty"`

	// AdditionalVolumeMounts is the additional volume mounts to use for
	// the node containers in this group.
	// +optional
	AdditionalVolumeMounts []corev1.VolumeMount `json:"additionalVolumeMounts,omitempty"`

	// AdditionalContainers is the additional containers to use for the
	// node pods in this group.
	// +optional
	AdditionalContainers []corev1.Container `json:"additionalContainers,omitempty"`

	// InitContainers is the init containers to use for the node pods in
	// this group.
	// +optional
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// Resources is the resource requirements for the node containers in
	// this group.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// ConfigGroup is the name of the configuration group from the Mesh
	// to use for this group. If not specified, the default configuration
	// will be used. Configurations can be further customized by specifying
	// a Config.
	// +optional
	ConfigGroup string `json:"configGroup,omitempty"`

	// Config is configuration overrides for this group.
	// +optional
	Config *NodeGroupConfig `json:"config,omitempty"`

	// Service is the configuration for exposing this group of nodes.
	// +optional
	Service *NodeGroupServiceConfig `json:"service,omitempty"`

	// PVCSpec is the specification for the PVCs to use for this group.
	// +optional
	PVCSpec *corev1.PersistentVolumeClaimSpec `json:"pvcSpec,omitempty"`
}

// NodeGroupConfig defines the desired Webmesh configurations for a group of nodes.
type NodeGroupConfig struct {
	// LogLevel is the log level to use for the node containers in this
	// group.
	// +kubebuilder:Validation:Enum:=debug;info;warn;error
	// +kubebuilder:default:="info"
	// +optional
	LogLevel string `json:"logLevel,omitempty"`

	// NoIPv6 is true if IPv6 should not be used for the node group.
	// +optional
	NoIPv6 bool `json:"noIPv6,omitempty"`
}

func (c *NodeGroupConfig) Default() {
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
}

// Merge merges the given NodeGroupConfig into this NodeGroupConfig. The
// given NodeGroupConfig takes precedence. The merged NodeGroupConfig is
// returned for convenience. If both are nil, a default NodeGroupConfig is
// returned.
func (c *NodeGroupConfig) Merge(in *NodeGroupConfig) *NodeGroupConfig {
	if in == nil && c == nil {
		var empty NodeGroupConfig
		empty.Default()
		return &empty
	}
	if in == nil {
		return c
	}
	if c == nil {
		return in
	}
	if in.LogLevel != "" {
		c.LogLevel = in.LogLevel
	}
	if in.NoIPv6 {
		c.NoIPv6 = true
	}
	return c
}

// NodeGroupServiceConfig defines the configurations for exposing a group of nodes.
type NodeGroupServiceConfig struct {
	// Type is the type of service to expose.
	// +kubebuilder:default:="ClusterIP"
	// +optional
	Type corev1.ServiceType `json:"type,omitempty"`

	// GRPCPort is the GRPC port to expose. This is used for communication
	// between clients and nodes.
	// +kubebuilder:default:=8443
	// +optional
	GRPCPort int32 `json:"grpcPort,omitempty"`

	// WireGuardPort is the starting WireGuard port to expose. This is used
	// for communication between nodes. Each node will have an external WireGuard
	// port assigned to it starting from this port.
	// +kubebuilder:default:=51820
	// +optional
	WireGuardPort int32 `json:"wireGuardPort,omitempty"`

	// Annotations are the annotations to use for the service.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// ExternalURL is the external URL to broadcast for this service.
	// If left unset it will be generated from the service IP.
	// +optional
	ExternalURL string `json:"externalURL,omitempty"`
}

func (c *NodeGroupServiceConfig) Default() {
	if c.Type == "" {
		c.Type = corev1.ServiceTypeClusterIP
	}
	if c.GRPCPort == 0 {
		c.GRPCPort = 8443
	}
	if c.WireGuardPort == 0 {
		c.WireGuardPort = 51820
	}
}

// NodeGroupStatus defines the observed state of NodeGroup
type NodeGroupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NodeGroup is the Schema for the nodegroups API
type NodeGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeGroupSpec   `json:"spec,omitempty"`
	Status NodeGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NodeGroupList contains a list of NodeGroup
type NodeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeGroup{}, &NodeGroupList{})
}
