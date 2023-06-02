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
	// A headless service is created for this group in addition to
	// any service defined in the NodeGroupServiceConfig.
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
func (c *Mesh) BootstrapGroup() *NodeGroup {
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
	bootstrapGroup := NodeGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GroupVersion.String(),
			Kind:       "NodeGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "bootstrap",
			Namespace:       c.GetNamespace(),
			Labels:          labels,
			Annotations:     c.GetAnnotations(),
			OwnerReferences: OwnerReferences(c),
		},
		Spec: c.Spec.Bootstrap,
	}
	bootstrapGroup.Annotations[BootstrapNodeGroupAnnotation] = "true"
	bootstrapGroup.Spec.Mesh = corev1.ObjectReference{
		APIVersion: c.APIVersion,
		Kind:       c.Kind,
		Name:       c.GetName(),
		Namespace:  c.GetNamespace(),
	}
	return &bootstrapGroup
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
