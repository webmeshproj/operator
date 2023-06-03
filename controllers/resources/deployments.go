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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	meshv1 "github.com/webmeshproj/operator/api/v1"
)

// NewNodeGroupLBDeployment returns a new Deployment for routing external traffic
// to a node group.
func NewNodeGroupLBDeployment(mesh *meshv1.Mesh, group *meshv1.NodeGroup, configChecksum string) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            meshv1.MeshNodeGroupLBName(mesh, group),
			Namespace:       group.GetNamespace(),
			Labels:          meshv1.NodeGroupLBLabels(mesh, group),
			OwnerReferences: meshv1.OwnerReferences(group),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: meshv1.NodeGroupLBSelector(mesh, group),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: meshv1.NodeGroupLBLabels(mesh, group),
					Annotations: map[string]string{
						meshv1.ConfigChecksumAnnotation: configChecksum,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "envoy",
							Image:           meshv1.DefaultNodeEnvoyImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args:            []string{"-c", "/etc/envoy/envoy.yaml"},
							Ports: func() []corev1.ContainerPort {
								ports := []corev1.ContainerPort{
									{
										Name:          "admin",
										ContainerPort: 9901,
										Protocol:      corev1.ProtocolTCP,
									},
									{
										Name:          "grpc",
										ContainerPort: meshv1.DefaultGRPCPort,
										Protocol:      corev1.ProtocolTCP,
									},
								}
								for i := 0; i < int(*group.Spec.Replicas); i++ {
									ports = append(ports,
										corev1.ContainerPort{
											Name:          fmt.Sprintf("wireguard-%d", i),
											ContainerPort: meshv1.DefaultWireGuardPort + int32(i),
											Protocol:      corev1.ProtocolUDP,
										})
								}
								return ports
							}(),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/envoy",
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								Privileged:               Pointer(false),
								ReadOnlyRootFilesystem:   Pointer(true),
								AllowPrivilegeEscalation: Pointer(false),
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: meshv1.MeshNodeGroupLBName(mesh, group),
									},
								},
							},
						},
					},
					ImagePullSecrets:              group.Spec.Cluster.ImagePullSecrets,
					TerminationGracePeriodSeconds: Pointer(int64(30)),
					NodeSelector:                  group.Spec.Cluster.NodeSelector,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:    Pointer(int64(65534)),
						RunAsGroup:   Pointer(int64(65534)),
						RunAsNonRoot: Pointer(true),
						FSGroup:      Pointer(int64(65534)),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Affinity:                  group.Spec.Cluster.Affinity,
					Tolerations:               group.Spec.Cluster.Tolerations,
					PreemptionPolicy:          group.Spec.Cluster.PreemptionPolicy,
					TopologySpreadConstraints: group.Spec.Cluster.TopologySpreadConstraints,
					ResourceClaims:            group.Spec.Cluster.ResourceClaims,
				},
			},
		},
	}
}
