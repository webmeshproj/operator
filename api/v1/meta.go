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
	"fmt"
	"strings"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// DefaultTLSKeyConfig is the default TLS key config certificates.
	DefaultTLSKeyConfig = certv1.CertificatePrivateKey{
		Algorithm: certv1.ECDSAKeyAlgorithm,
		Size:      384,
	}
)

// OwnerReferences returns the owner references for the given object.
func OwnerReferences(obj client.Object) []metav1.OwnerReference {
	ref := metav1.NewControllerRef(obj, obj.GetObjectKind().GroupVersionKind())
	ref.BlockOwnerDeletion = &[]bool{true}[0]
	return []metav1.OwnerReference{*ref}
}

// MeshSelfSignerName returns the name of the self-signer for the given Mesh.
func MeshSelfSignerName(mesh *Mesh) string {
	return fmt.Sprintf("%s-self-signer", mesh.GetName())
}

// MeshCAName returns the name of the CA for the given Mesh.
func MeshCAName(mesh *Mesh) string {
	return fmt.Sprintf("%s-ca", mesh.GetName())
}

// MeshCAHostname returns the hostname for the given Mesh CA.
func MeshCAHostname(mesh *Mesh) string {
	return fmt.Sprintf("%s-ca.webmesh.internal", mesh.GetName())
}

// MeshAdminCertName returns the name of the admin certificate for the given Mesh.
func MeshAdminCertName(mesh *Mesh) string {
	return fmt.Sprintf("%s-admin", mesh.GetName())
}

// MeshAdminConfigName returns the name of the admin config for the given Mesh.
func MeshAdminConfigName(mesh *Mesh) string {
	return fmt.Sprintf("%s-admin-config", mesh.GetName())
}

// MeshAdminHostname returns the hostname for the given Mesh admin.
func MeshAdminHostname(mesh *Mesh) string {
	return fmt.Sprintf("%s-admin", mesh.GetName())
}

// MeshSelfSignerRef returns a reference to the self-signer for the given Mesh.
func MeshSelfSignerRef(mesh *Mesh) cmmeta.ObjectReference {
	return cmmeta.ObjectReference{
		Kind: "Issuer",
		Name: MeshSelfSignerName(mesh),
	}
}

// MeshBootstrapGroupName returns the name of the bootstrap group for the given Mesh.
func MeshBootstrapGroupName(mesh *Mesh) string {
	return fmt.Sprintf("%s-bootstrap", mesh.GetName())
}

// MeshBootstrapLBGroupName returns the name of the bootstrap load balancer group for the given Mesh.
func MeshBootstrapLBGroupName(mesh *Mesh) string {
	return fmt.Sprintf("%s-bootstrap-lb", mesh.GetName())
}

// MeshNodeCertName returns the name of the node certificate for the given Mesh.
func MeshNodeCertName(mesh *Mesh, group *NodeGroup, index int) string {
	return MeshNodeGroupPodName(mesh, group, index)
}

// MeshNodeHostname returns the hostname for the given Mesh node.
func MeshNodeHostname(mesh *Mesh, group *NodeGroup, index int) string {
	return MeshNodeGroupPodName(mesh, group, index)
}

// MeshNodeDNSNames returns the DNS names for the given Mesh node.
func MeshNodeDNSNames(mesh *Mesh, group *NodeGroup, index int) []string {
	svcName := MeshNodeGroupHeadlessServiceName(mesh, group)
	podName := MeshNodeGroupPodName(mesh, group, index)
	return []string{
		// Service Names
		svcName,
		fmt.Sprintf("%s.%s", svcName, group.GetNamespace()),
		fmt.Sprintf("%s.%s.svc", svcName, group.GetNamespace()),
		MeshNodeGroupHeadlessServiceFQDN(mesh, group),
		// Pod Names
		fmt.Sprintf("%s.%s", podName, svcName),
		fmt.Sprintf("%s.%s.%s", podName, svcName, group.GetNamespace()),
		fmt.Sprintf("%s.%s.%s.svc", podName, svcName, group.GetNamespace()),
		MeshNodeClusterFQDN(mesh, group, index),
	}
}

// MeshNodeGroupHeadlessServiceFQDN returns the cluster FQDN for the given Mesh node group's
// headless service.
func MeshNodeGroupHeadlessServiceFQDN(mesh *Mesh, group *NodeGroup) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local",
		MeshNodeGroupHeadlessServiceName(mesh, group),
		group.GetNamespace())
}

// MeshNodeClusterFQDN returns the cluster FQDN for the given Mesh node.
func MeshNodeClusterFQDN(mesh *Mesh, group *NodeGroup, index int) string {
	return fmt.Sprintf("%s.%s",
		MeshNodeGroupPodName(mesh, group, index),
		MeshNodeGroupHeadlessServiceFQDN(mesh, group))
}

// MeshNodeGroupStatefulSetName returns the name of the StatefulSet for the given Mesh node group.
func MeshNodeGroupStatefulSetName(mesh *Mesh, group *NodeGroup) string {
	if strings.HasPrefix(group.GetName(), mesh.GetName()) {
		return group.GetName()
	}
	return fmt.Sprintf("%s-%s", mesh.GetName(), group.GetName())
}

// MeshNodeGroupPodName returns the name of the Pod for the given Mesh node group.
func MeshNodeGroupPodName(mesh *Mesh, group *NodeGroup, index int) string {
	return fmt.Sprintf("%s-%d", MeshNodeGroupStatefulSetName(mesh, group), index)
}

// MeshNodeGroupLBName returns the name of the LB Service for the given Mesh node group.
func MeshNodeGroupLBName(mesh *Mesh, group *NodeGroup) string {
	return fmt.Sprintf("%s-public", MeshNodeGroupStatefulSetName(mesh, group))
}

// MeshNodeGroupConfigMapName returns the name of the ConfigMap for the given Mesh node group.
func MeshNodeGroupConfigMapName(mesh *Mesh, group *NodeGroup) string {
	return MeshNodeGroupStatefulSetName(mesh, group)
}

// MeshNodeGroupHeadlessServiceName returns the name of the headless Service for the given Mesh node group.
func MeshNodeGroupHeadlessServiceName(mesh *Mesh, group *NodeGroup) string {
	return MeshNodeGroupStatefulSetName(mesh, group)
}

// MeshLabels returns the labels for the given Mesh.
func MeshLabels(mesh *Mesh) map[string]string {
	labels := mesh.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	for k, v := range MeshSelector(mesh) {
		labels[k] = v
	}
	return labels
}

// MeshSelector returns the selector for the given Mesh.
func MeshSelector(mesh *Mesh) map[string]string {
	return map[string]string{
		MeshNameLabel:      mesh.GetName(),
		MeshNamespaceLabel: mesh.GetNamespace(),
	}
}

// NodeGroupLabels returns the labels for the given Mesh node group.
func NodeGroupLabels(mesh *Mesh, group *NodeGroup) map[string]string {
	labels := MeshLabels(mesh)
	for k, v := range NodeGroupSelector(mesh, group) {
		labels[k] = v
	}
	return labels
}

// NodeGroupSelector returns the selector for the given Mesh node group.
func NodeGroupSelector(mesh *Mesh, group *NodeGroup) map[string]string {
	labels := MeshSelector(mesh)
	labels[NodeGroupNameLabel] = group.GetName()
	labels[NodeGroupNamespaceLabel] = group.GetNamespace()
	return labels
}

// MeshBootstrapGroupSelector returns the selector for a Mesh's bootstrap node group.
func MeshBootstrapGroupSelector(mesh *Mesh) map[string]string {
	return map[string]string{
		MeshNameLabel:           mesh.GetName(),
		MeshNamespaceLabel:      mesh.GetNamespace(),
		BootstrapNodeGroupLabel: "true",
	}
}
