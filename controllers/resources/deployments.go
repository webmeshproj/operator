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
	"k8s.io/apimachinery/pkg/util/intstr"

	meshv1 "github.com/webmeshproj/operator/api/v1"
)

// NewNodeGroupLBDeployment returns a new load balancer deployment for a NodeGroup.
func NewNodeGroupLBDeployment(mesh *meshv1.Mesh, group *meshv1.NodeGroup, configCsum string) *appsv1.Deployment {
	groupspec := group.Spec.Cluster
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            meshv1.MeshNodeGroupLBName(mesh, group),
			Namespace:       group.GetNamespace(),
			Labels:          meshv1.NodeGroupLabels(mesh, group),
			Annotations:     group.GetAnnotations(),
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
						meshv1.ConfigChecksumAnnotation: configCsum,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "traefik",
							Image:           meshv1.DefaultNodeTraefikImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args: func() []string {
								args := []string{
									"--ping",
									"--ping.entrypoint=traefik",
									"--log.level=INFO",
									"--providers.file.directory=/etc/traefik",
									"--entrypoints.traefik.address=:9000",
									fmt.Sprintf("--entrypoints.grpc.address=:%d", meshv1.DefaultGRPCPort),
								}
								for i := 0; i < int(*group.Spec.Replicas); i++ {
									args = append(args,
										fmt.Sprintf("--entrypoints.wireguard-%d.address=:%d/udp", i, meshv1.DefaultWireGuardPort+i),
										fmt.Sprintf("--entrypoints.wireguard-%d.udp.timeout=60", i),
									)
								}
								return args
							}(),
							Ports: func() []corev1.ContainerPort {
								ports := []corev1.ContainerPort{
									{
										Name:          "traefik",
										ContainerPort: 9000,
										Protocol:      corev1.ProtocolTCP,
									},
									{
										Name:          "grpc",
										ContainerPort: meshv1.DefaultGRPCPort,
										Protocol:      corev1.ProtocolTCP,
									},
								}
								for i := 0; i < int(*group.Spec.Replicas); i++ {
									ports = append(ports, corev1.ContainerPort{
										Name:          fmt.Sprintf("wireguard-%d", i),
										ContainerPort: int32(meshv1.DefaultWireGuardPort + i),
										Protocol:      corev1.ProtocolUDP,
									})
								}
								return ports
							}(),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/traefik",
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: 10,
								TimeoutSeconds:      5,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/ping",
										Port: intstr.FromString("traefik"),
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								InitialDelaySeconds: 10,
								TimeoutSeconds:      5,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/ping",
										Port: intstr.FromString("traefik"),
									},
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
					ImagePullSecrets: groupspec.ImagePullSecrets,
					NodeSelector:     groupspec.NodeSelector,
					HostNetwork:      groupspec.HostNetwork,
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
		},
	}
}
