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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	meshv1 "github.com/webmeshproj/operator/api/v1"
)

// NewNodeGroupHeadlessService returns a new headless service for a NodeGroup.
func NewNodeGroupHeadlessService(mesh *meshv1.Mesh, group *meshv1.NodeGroup, ownedBy client.Object) *corev1.Service {
	policy := corev1.IPFamilyPolicyPreferDualStack
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            meshv1.MeshNodeGroupHeadlessServiceName(mesh, group),
			Namespace:       group.GetNamespace(),
			Labels:          meshv1.NodeGroupLabels(mesh, group),
			OwnerReferences: meshv1.OwnerReferences(ownedBy),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP:      "None",
			Type:           corev1.ServiceTypeClusterIP,
			IPFamilyPolicy: &policy,
			Selector:       meshv1.NodeGroupSelector(mesh, group),
			Ports: []corev1.ServicePort{
				{
					Name:       "grpc",
					Port:       meshv1.DefaultGRPCPort,
					TargetPort: intstr.FromString("grpc"),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "raft",
					Port:       meshv1.DefaultRaftPort,
					TargetPort: intstr.FromString("raft"),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "wireguard",
					Port:       meshv1.DefaultWireGuardPort,
					TargetPort: intstr.FromString("wireguard"),
					Protocol:   corev1.ProtocolUDP,
				},
			},
		},
	}
}

// NewNodeGroupLBService returns a new service for exposing a NodeGroup.
func NewNodeGroupLBService(mesh *meshv1.Mesh, group *meshv1.NodeGroup, ownedBy client.Object) *corev1.Service {
	policy := corev1.IPFamilyPolicyPreferDualStack
	spec := group.Spec.Service
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            meshv1.MeshNodeGroupLBName(mesh, group),
			Namespace:       group.GetNamespace(),
			Labels:          meshv1.NodeGroupLBLabels(mesh, group),
			OwnerReferences: meshv1.OwnerReferences(ownedBy),
			Annotations:     spec.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:           spec.Type,
			IPFamilyPolicy: &policy,
			Selector:       meshv1.NodeGroupLBSelector(mesh, group),
			Ports: func() []corev1.ServicePort {
				ports := []corev1.ServicePort{
					{
						Name:       "grpc",
						Port:       spec.GRPCPort,
						TargetPort: intstr.FromString("grpc"),
						Protocol:   corev1.ProtocolTCP,
					},
				}
				for i := 0; i < int(group.Spec.Replicas); i++ {
					ports = append(ports, corev1.ServicePort{
						Name:       fmt.Sprintf("wireguard-%d", i),
						Port:       spec.WireGuardPort + int32(i),
						TargetPort: intstr.FromInt(int(spec.WireGuardPort) + i),
						Protocol:   corev1.ProtocolUDP,
					})
				}
				return ports
			}(),
		},
	}
}
