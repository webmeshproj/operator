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

package resources

import (
	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	meshv1 "github.com/webmeshproj/operator/api/v1"
)

// NewMeshCACertificate returns a new CA certificate for a Mesh.
func NewMeshCACertificate(mesh *meshv1.Mesh) *certv1.Certificate {
	return &certv1.Certificate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: certv1.SchemeGroupVersion.String(),
			Kind:       "Certificate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: meshv1.MeshCAName(mesh),
			Namespace: func() string {
				if mesh.Spec.Issuer.Kind == "ClusterIssuer" {
					return "cert-manager"
				}
				return mesh.GetNamespace()
			}(),
			Labels:          meshv1.MeshLabels(mesh),
			OwnerReferences: meshv1.OwnerReferences(mesh),
		},
		Spec: certv1.CertificateSpec{
			CommonName: meshv1.MeshCAHostname(mesh),
			SecretName: meshv1.MeshCAName(mesh),
			IsCA:       true,
			PrivateKey: &meshv1.DefaultTLSKeyConfig,
			IssuerRef:  meshv1.MeshSelfSignerRef(mesh),
		},
	}
}

// NewNodeCertificate returns a new TLS certificate for a Mesh node.
func NewNodeCertificate(mesh *meshv1.Mesh, nodeGroup *meshv1.NodeGroup, index int) *certv1.Certificate {
	return &certv1.Certificate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: certv1.SchemeGroupVersion.String(),
			Kind:       "Certificate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            meshv1.MeshNodeCertName(mesh, nodeGroup, index),
			Namespace:       nodeGroup.GetNamespace(),
			Labels:          meshv1.NodeGroupLabels(mesh, nodeGroup),
			OwnerReferences: meshv1.OwnerReferences(mesh),
		},
		Spec: certv1.CertificateSpec{
			CommonName: meshv1.MeshNodeHostname(mesh, nodeGroup, index),
			SecretName: meshv1.MeshNodeCertName(mesh, nodeGroup, index),
			DNSNames:   meshv1.MeshNodeDNSNames(mesh, nodeGroup, index),
			Usages: []certv1.KeyUsage{
				certv1.UsageDigitalSignature,
				certv1.UsageKeyEncipherment,
				certv1.UsageServerAuth,
				certv1.UsageClientAuth,
			},
			PrivateKey: &meshv1.DefaultTLSKeyConfig,
			IssuerRef:  mesh.IssuerReference(),
		},
	}
}
