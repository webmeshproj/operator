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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	meshv1 "github.com/webmeshproj/operator/api/v1"
)

// NewNodeGroupStatefulSet returns a new statefulset for a NodeGroup.
func NewNodeGroupStatefulSet(mesh *meshv1.Mesh, group *meshv1.NodeGroup, configChecksum string) *appsv1.StatefulSet {
	groupspec := group.Spec.Cluster
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            meshv1.MeshNodeGroupStatefulSetName(mesh, group),
			Namespace:       group.GetNamespace(),
			Labels:          meshv1.NodeGroupLabels(mesh, group),
			Annotations:     group.GetAnnotations(),
			OwnerReferences: meshv1.OwnerReferences(group),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: group.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: meshv1.NodeGroupSelector(mesh, group),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: meshv1.NodeGroupLabels(mesh, group),
					Annotations: map[string]string{
						meshv1.ConfigChecksumAnnotation: configChecksum,
					},
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: groupspec.ImagePullSecrets,
					InitContainers:   groupspec.InitContainers,
					Containers: append([]corev1.Container{
						{
							Name:            "node",
							Image:           group.Spec.Image,
							ImagePullPolicy: groupspec.ImagePullPolicy,
							Args:            []string{"--config", "/etc/webmesh/config.yaml"},
							Env: []corev1.EnvVar{
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "grpc",
									ContainerPort: meshv1.DefaultGRPCPort,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "raft",
									ContainerPort: meshv1.DefaultRaftPort,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "wireguard",
									ContainerPort: meshv1.DefaultWireGuardPort,
									Protocol:      corev1.ProtocolUDP,
								},
							},
							VolumeMounts: func() []corev1.VolumeMount {
								vols := []corev1.VolumeMount{
									{
										Name:      "config",
										MountPath: "/etc/webmesh",
									},
									{
										Name:      "data",
										MountPath: meshv1.DefaultDataDirectory,
									},
								}
								for i := 0; i < int(*group.Spec.Replicas); i++ {
									vols = append(vols, corev1.VolumeMount{
										Name:      fmt.Sprintf("node-tls-%d", i),
										MountPath: fmt.Sprintf("%s/%s", meshv1.DefaultTLSDirectory, meshv1.MeshNodeGroupPodName(mesh, group, i)),
									})
								}
								return append(vols, groupspec.AdditionalVolumeMounts...)
							}(),
							Resources: groupspec.Resources,
							SecurityContext: &corev1.SecurityContext{
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{
										"NET_ADMIN",
										"NET_RAW",
										"SYS_MODULE",
									},
								},
								RunAsUser:    Pointer(int64(0)),
								RunAsGroup:   Pointer(int64(0)),
								Privileged:   Pointer(true),
								RunAsNonRoot: Pointer(false),
								SeccompProfile: &corev1.SeccompProfile{
									Type: corev1.SeccompProfileTypeRuntimeDefault,
								},
							},
						},
					}, groupspec.AdditionalContainers...),
					Volumes: func() []corev1.Volume {
						vols := []corev1.Volume{
							{
								Name: "config",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: meshv1.MeshNodeGroupConfigMapName(mesh, group),
										},
									},
								},
							},
						}
						for i := 0; i < int(*group.Spec.Replicas); i++ {
							vols = append(vols, corev1.Volume{
								Name: fmt.Sprintf("node-tls-%d", i),
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: meshv1.MeshNodeCertName(mesh, group, i),
									},
								},
							})
						}
						if groupspec.PVCSpec == nil {
							vols = append(vols, corev1.Volume{
								Name: "data",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							})
						}
						return append(vols, groupspec.AdditionalVolumes...)
					}(),
					TerminationGracePeriodSeconds: Pointer(int64(60)),
					NodeSelector:                  groupspec.NodeSelector,
					HostNetwork:                   groupspec.HostNetwork,
					// Make sure additional user-defined containers run
					// with lower privileges unless configured otherwise.
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:    Pointer(int64(65534)),
						RunAsGroup:   Pointer(int64(65534)),
						RunAsNonRoot: Pointer(true),
						FSGroup:      Pointer(int64(65534)),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Affinity:                  groupspec.Affinity,
					Tolerations:               groupspec.Tolerations,
					PreemptionPolicy:          groupspec.PreemptionPolicy,
					TopologySpreadConstraints: groupspec.TopologySpreadConstraints,
					ResourceClaims:            groupspec.ResourceClaims,
				},
			},
			VolumeClaimTemplates: func() []corev1.PersistentVolumeClaim {
				if group.Spec.Cluster.PVCSpec == nil {
					return nil
				}
				return []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "data",
						},
						Spec: *group.Spec.Cluster.PVCSpec,
					},
				}
			}(),
			ServiceName:         meshv1.MeshNodeGroupHeadlessServiceName(mesh, group),
			PodManagementPolicy: appsv1.ParallelPodManagement,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					MaxUnavailable: Pointer(intstr.FromInt(1)),
				},
			},
			PersistentVolumeClaimRetentionPolicy: &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenDeleted: appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
				WhenScaled:  appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
			},
		},
	}
}
