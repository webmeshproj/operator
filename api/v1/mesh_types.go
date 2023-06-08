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
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MeshSpec defines the desired state of Mesh
type MeshSpec struct {
	// Image is the default image to use for configurations if not
	// specified otherwise.
	// +kubebuilder:default:="ghcr.io/webmeshproj/node:latest"
	// +optional
	Image string `json:"image,omitempty"`

	// ConfigGroups is a map of configurations for groups of nodes.
	// These can be referenced by name in NodeGroupSpecs.
	// +optional
	ConfigGroups map[string]NodeGroupConfig `json:"configGroups,omitempty"`

	// Bootstrap is the configuration for the bootstrap node group.
	// A headless service is created for this group that is only accessible
	// within the cluster. If an exposed service is configured, an additional
	// load balancer node group will be created as an initial entrypoint to
	// the mesh.
	// +optional
	Bootstrap NodeGroupSpec `json:"bootstrap,omitempty"`

	// IPv4 is the IPv4 CIDR to use for the mesh. This cannot be
	// changed after creation.
	// +kubebuilder:default:="172.16.0.0/12"
	// +optional
	IPv4 string `json:"ipv4,omitempty"`

	// Issuer is the configuration for issuing TLS certificates.
	// +optional
	Issuer IssuerConfig `json:"issuer,omitempty"`
}

// IssuerConfig defines the configuration for issuing TLS certificates.
type IssuerConfig struct {
	// Create is true if the issuer should be created.
	// +optional
	Create bool `json:"create,omitempty"`

	// Kind is the kind of issuer to create.
	// +kubebuilder:default:="Issuer"
	// +optional
	Kind string `json:"type,omitempty"`

	// IssuerRef is the reference to an existing issuer to use.
	// +optional
	IssuerRef cmmeta.ObjectReference `json:"issuerRef,omitempty"`
}

// BootstrapGroup returns a NodeGroup for the bootstrap group.
func (c *Mesh) BootstrapGroups() []*NodeGroup {
	if c == nil {
		return nil
	}
	labels := c.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	for k, v := range MeshBootstrapGroupSelector(c) {
		labels[k] = v
	}
	annotations := c.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[BootstrapNodeGroupAnnotation] = "true"
	spec := c.Spec.Bootstrap.DeepCopy()
	if spec.Config == nil {
		spec.Config = &NodeGroupConfig{}
	}
	if spec.Config.Services == nil {
		spec.Config.Services = &NodeServicesConfig{}
	}
	// Force the admin api, mesh api, and leader proxy on the bootstrap groups
	spec.Config.Services.EnableAdminAPI = true
	spec.Config.Services.EnableMeshAPI = true
	spec.Config.Services.EnableLeaderProxy = true
	bootstrapGroup := NodeGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GroupVersion.String(),
			Kind:       "NodeGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            MeshBootstrapGroupName(c),
			Namespace:       c.GetNamespace(),
			Labels:          labels,
			Annotations:     annotations,
			OwnerReferences: OwnerReferences(c),
		},
		Spec: *spec,
	}
	if bootstrapGroup.Spec.Cluster.Service != nil {
		// Force this to be nil, we expose via a separate node group.
		bootstrapGroup.Spec.Cluster.Service = nil
	}
	bootstrapGroup.Spec.Mesh = corev1.ObjectReference{
		APIVersion: c.APIVersion,
		Kind:       c.Kind,
		Name:       c.GetName(),
		Namespace:  c.GetNamespace(),
	}
	groups := []*NodeGroup{&bootstrapGroup}
	// Create an LB group if we are exposing the bootstrap group.
	if c.Spec.Bootstrap.Cluster.Service != nil {
		lbGroup := bootstrapGroup.DeepCopy()
		lbGroup.SetName(MeshBootstrapLBGroupName(c))
		// This is not a bootstrap group, it joins the initial group
		delete(lbGroup.Annotations, BootstrapNodeGroupAnnotation)
		// But we give it the same zone awareness as the bootstrap group
		if lbGroup.Labels == nil {
			lbGroup.Labels = map[string]string{}
		}
		lbGroup.Labels[ZoneAwarenessLabel] = bootstrapGroup.GetName()
		// We only run a single replica of the load balancer group
		lbGroup.Spec.Replicas = nil
		lbGroup.Spec.Config.Voter = true
		lbGroup.Spec.Cluster.Service = c.Spec.Bootstrap.Cluster.Service
		groups = append(groups, lbGroup)
	}
	return groups
}

// IssuerReference returns the issuer reference for the mesh.
func (c *Mesh) IssuerReference() cmmeta.ObjectReference {
	if c == nil {
		return cmmeta.ObjectReference{}
	}
	if c.Spec.Issuer.Create {
		return cmmeta.ObjectReference{
			Kind: c.Spec.Issuer.Kind,
			Name: MeshCAName(c),
		}
	}
	return c.Spec.Issuer.IssuerRef
}

// MeshStatus defines the observed state of Mesh
type MeshStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Mesh is the Schema for the meshes API
type Mesh struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MeshSpec   `json:"spec,omitempty"`
	Status MeshStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MeshList contains a list of Mesh
type MeshList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Mesh `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Mesh{}, &MeshList{})
}
