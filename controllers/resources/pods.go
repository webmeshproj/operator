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
	"crypto/sha256"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	meshv1 "github.com/webmeshproj/operator/api/v1"
)

func NewNodeGroupPod(mesh *meshv1.Mesh, group *meshv1.NodeGroup, confChecksum string, index int) (*corev1.Pod, error) {
	groupspec := group.Spec.Cluster
	podspec := corev1.PodSpec{
		Hostname:         meshv1.MeshNodeHostname(mesh, group, index),
		Subdomain:        meshv1.MeshNodeGroupHeadlessServiceName(mesh, group),
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
						ContainerPort: meshv1.DefaultWireGuardPort + int32(index),
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
							Name:      "node-tls",
							MountPath: meshv1.DefaultTLSDirectory,
						},
						{
							Name:      "data",
							MountPath: meshv1.DefaultDataDirectory,
						},
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
								Name: meshv1.MeshNodeGroupPodName(mesh, group, index),
							},
						},
					},
				},
				{
					Name: "node-tls",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: meshv1.MeshNodeCertName(mesh, group, index),
						},
					},
				},
			}
			if groupspec.PVCSpec == nil {
				vols = append(vols, corev1.Volume{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				})
			} else {
				vols = append(vols, corev1.Volume{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: meshv1.MeshNodeGroupPodName(mesh, group, index),
						},
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
	}
	annotations := group.Spec.Cluster.PodAnnotations
	if annotations == nil {
		annotations = make(map[string]string)
	}
	specJSON, err := json.Marshal(&podspec)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(specJSON))
	annotations[meshv1.SpecChecksumAnnotation] = sum
	annotations[meshv1.ConfigChecksumAnnotation] = confChecksum
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            meshv1.MeshNodeGroupPodName(mesh, group, index),
			Namespace:       group.GetNamespace(),
			Labels:          meshv1.NodeGroupLabels(mesh, group),
			Annotations:     annotations,
			OwnerReferences: meshv1.OwnerReferences(group),
		},
		Spec: podspec,
	}, nil
}
