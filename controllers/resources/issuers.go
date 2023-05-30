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
	"sigs.k8s.io/controller-runtime/pkg/client"

	meshv1 "github.com/webmeshproj/operator/api/v1"
)

// NewMeshSelfSigner returns a new self-signer for a Mesh.
func NewMeshSelfSigner(mesh *meshv1.Mesh) *certv1.Issuer {
	return &certv1.Issuer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: certv1.SchemeGroupVersion.String(),
			Kind:       "Issuer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: meshv1.MeshSelfSignerName(mesh),
			Namespace: func() string {
				if mesh.Spec.Issuer.Kind == "ClusterIssuer" {
					return "cert-manager"
				}
				return mesh.GetNamespace()
			}(),
			Labels:          meshv1.MeshLabels(mesh),
			OwnerReferences: meshv1.OwnerReferences(mesh),
		},
		Spec: certv1.IssuerSpec{
			IssuerConfig: certv1.IssuerConfig{
				SelfSigned: &certv1.SelfSignedIssuer{},
			},
		},
	}
}

// NewMeshIssuer returns a new issuer for a Mesh.
func NewMeshIssuer(mesh *meshv1.Mesh) client.Object {
	objectMeta := metav1.ObjectMeta{
		Name:            meshv1.MeshCAName(mesh),
		Labels:          meshv1.MeshLabels(mesh),
		OwnerReferences: meshv1.OwnerReferences(mesh),
	}
	typeMeta := metav1.TypeMeta{
		APIVersion: certv1.SchemeGroupVersion.String(),
		Kind:       mesh.Spec.Issuer.Kind,
	}
	spec := certv1.IssuerSpec{
		IssuerConfig: certv1.IssuerConfig{
			CA: &certv1.CAIssuer{
				SecretName: meshv1.MeshCAName(mesh),
			},
		},
	}
	if mesh.Spec.Issuer.Kind == "ClusterIssuer" {
		return &certv1.ClusterIssuer{
			TypeMeta:   typeMeta,
			ObjectMeta: objectMeta,
			Spec:       spec,
		}
	}
	objectMeta.Namespace = mesh.GetNamespace()
	return &certv1.Issuer{
		TypeMeta:   typeMeta,
		ObjectMeta: objectMeta,
		Spec:       spec,
	}
}
