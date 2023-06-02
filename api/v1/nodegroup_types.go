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
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// NodeGroupSpec is the specification for a group of nodes.
type NodeGroupSpec struct {
	// Image is the image to use for the node.
	// +kubebuilder:default:="ghcr.io/webmeshproj/node:latest"
	// +optional
	Image string `json:"image,omitempty"`

	// Replicas is the number of replicas to run for this group.
	// +kubebuilder:default:=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Mesh is a reference to the Mesh this group belongs to.
	// +optional
	Mesh corev1.ObjectReference `json:"mesh,omitempty"`

	// ConfigGroup is the name of the configuration group from the Mesh
	// to use for this group. If not specified, the default configuration
	// will be used. Configurations can be further customized by specifying
	// a Config.
	// +optional
	ConfigGroup string `json:"configGroup,omitempty"`

	// Config is configuration overrides for this group.
	// +optional
	Config *NodeGroupConfig `json:"config,omitempty"`

	// Cluster is the configuration for a group of nodes running in a
	// Kubernetes cluster.
	// +optional
	Cluster *NodeGroupClusterConfig `json:"cluster,omitempty"`

	// GoogleCloud is the configuration for a group of nodes running in
	// Google Cloud.
	// +optional
	GoogleCloud *NodeGroupGoogleCloudConfig `json:"googleCloud,omitempty"`
}

func (n *NodeGroupSpec) Default() {
	if n.Replicas == nil {
		n.Replicas = new(int32)
		*n.Replicas = 1
	}
	if n.ConfigGroup == "" && n.Config == nil {
		n.Config = &NodeGroupConfig{}
		n.Config.Default()
	} else if n.Config != nil {
		n.Config.Default()
	}

	if n.Cluster == nil {
		if n.GoogleCloud == nil {
			n.Cluster = &NodeGroupClusterConfig{}
			n.Cluster.Default()
		}
	}
}

// NodeGroupClusterConfig is the configuration for a group of nodes running in
// a Kubernetes cluster.
type NodeGroupClusterConfig struct {
	// ImagePullPolicy is the image pull policy to use for the node.
	// +kubebuilder:default:="IfNotPresent"
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// ImagePullSecrets is the list of image pull secrets to use for the
	// node.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

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
	// +optional
	PreemptionPolicy *corev1.PreemptionPolicy `json:"preemptionPolicy,omitempty"`

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

	// Service is the configuration for exposing this group of nodes.
	// +optional
	Service *NodeGroupLBConfig `json:"service,omitempty"`

	// PVCSpec is the specification for the PVCs to use for this group.
	// +optional
	PVCSpec *corev1.PersistentVolumeClaimSpec `json:"pvcSpec,omitempty"`

	// Kubeconfig is a reference to a secret containing a kubeconfig to use
	// for this group. If not specified, the current kubeconfig will be used.
	// +optional
	Kubeconfig *corev1.SecretKeySelector `json:"kubeconfig,omitempty"`
}

// Default sets default values for the configuration.
func (c *NodeGroupClusterConfig) Default() {
	if c.ImagePullPolicy == "" {
		c.ImagePullPolicy = corev1.PullIfNotPresent
	}
	if c.Service != nil {
		c.Service.Default()
	}
}

// NodeGroupLBConfig defines the configurations for exposing a group of nodes.
type NodeGroupLBConfig struct {
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

func (c *NodeGroupLBConfig) Default() {
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

// NodeGroupGoogleCloudConfig defines the desired configurations for a node group
// running on Google Cloud compute instances.
type NodeGroupGoogleCloudConfig struct {
	// ProjectID is the ID of the Google Cloud project.
	// +optional
	ProjectID string `json:"projectID,omitempty"`

	// Subnetwork is the name of the subnetwork to place the WAN interface.
	// +kubebuilder:validation:Required
	Subnetwork string `json:"subnetwork"`

	// Region is the region where the router resides.
	// +optional
	Region string `json:"region,omitempty"`

	// Zone is the zone where the router resides.
	// +kubebuilder:validation:Required
	Zone string `json:"zone"`

	// MachineType is the machine type of the router.
	// +kubebuilder:validation:Required
	MachineType string `json:"machineType"`

	// Tags is a list of instance tags to which this router applies.
	// +optional
	Tags []string `json:"tags,omitempty"`

	// Credentials is the credentials to use for the Google Cloud API.
	// If omitted, workload identity will be used.
	// +optional
	Credentials *corev1.SecretKeySelector `json:"credentials,omitempty"`
}

func (c *NodeGroupGoogleCloudConfig) Validate(path *field.Path) error {
	if c.ProjectID == "" {
		return field.Invalid(path.Child("projectID"), c.ProjectID, "projectID is required")
	}
	if c.Subnetwork == "" {
		return field.Invalid(path.Child("subnetwork"), c.Subnetwork, "subnetwork is required")
	}
	if c.Zone == "" {
		return field.Invalid(path.Child("zone"), c.Zone, "zone is required")
	}
	if c.MachineType == "" {
		return field.Invalid(path.Child("machineType"), c.MachineType, "machineType is required")
	}
	return nil
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
