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
	"bytes"
	"crypto/sha256"
	"fmt"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	meshv1 "github.com/webmeshproj/operator/api/v1"
	"github.com/webmeshproj/operator/controllers/nodeconfig"
)

// NewNodeGroupConfigMap returns a new ConfigMap for a NodeGroup.
func NewNodeGroupConfigMap(mesh *meshv1.Mesh, group *meshv1.NodeGroup, conf *nodeconfig.Config, index int) (cm *corev1.ConfigMap) {
	annotations := group.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[meshv1.ConfigChecksumAnnotation] = conf.Checksum()
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            meshv1.MeshNodeGroupPodName(mesh, group, index),
			Namespace:       group.GetNamespace(),
			Labels:          meshv1.NodeGroupLabels(mesh, group),
			Annotations:     annotations,
			OwnerReferences: meshv1.OwnerReferences(group),
		},
		Data: map[string]string{
			"config.yaml": string(conf.Raw()),
		},
	}
}

// NewNodeGroupLBConfigMap returns a new ConfigMap for a NodeGroup load balancer.
func NewNodeGroupLBConfigMap(mesh *meshv1.Mesh, group *meshv1.NodeGroup) (cm *corev1.ConfigMap, csum string, err error) {
	conf := traefikConfig{
		TCP: tcpConfig{
			Routers: map[string]routerConfig{
				"grpc": {
					EntryPoints: []string{"grpc"},
					Service:     "grpc",
					Rule:        "HostSNI(`*`)",
					TLS: tlsConfig{
						Passthrough: true,
					},
				},
			},
			Services: map[string]serviceConfig{
				"grpc": {
					LoadBalancer: loadBalancerConfig{
						Servers: func() []serverConfig {
							confs := make([]serverConfig, *group.Spec.Replicas)
							for i := 0; i < int(*group.Spec.Replicas); i++ {
								confs[i] = serverConfig{
									Address: fmt.Sprintf("%s:%d",
										meshv1.MeshNodeClusterFQDN(mesh, group, i),
										meshv1.DefaultGRPCPort),
								}
							}
							return confs
						}(),
					},
				},
			},
		},
		UDP: udpConfig{
			Routers: func() map[string]routerConfig {
				out := make(map[string]routerConfig)
				for i := 0; i < int(*group.Spec.Replicas); i++ {
					out[fmt.Sprintf("wireguard-%d", i)] = routerConfig{
						EntryPoints: []string{fmt.Sprintf("wireguard-%d", i)},
						Service:     fmt.Sprintf("wireguard-%d", i),
					}
				}
				return out
			}(),
			Services: func() map[string]serviceConfig {
				out := make(map[string]serviceConfig)
				for i := 0; i < int(*group.Spec.Replicas); i++ {
					out[fmt.Sprintf("wireguard-%d", i)] = serviceConfig{
						LoadBalancer: loadBalancerConfig{
							Servers: []serverConfig{
								{
									Address: fmt.Sprintf("%s:%d",
										meshv1.MeshNodeClusterFQDN(mesh, group, i),
										meshv1.DefaultWireGuardPort+i),
								},
							},
						},
					}
				}
				return out
			}(),
		},
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	err = enc.Encode(conf)
	if err != nil {
		return nil, "", err
	}
	csum = fmt.Sprintf("%x", sha256.Sum256(buf.Bytes()))
	annotations := group.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[meshv1.ConfigChecksumAnnotation] = csum
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            meshv1.MeshNodeGroupLBName(mesh, group),
			Namespace:       group.GetNamespace(),
			Labels:          meshv1.NodeGroupLabels(mesh, group),
			Annotations:     annotations,
			OwnerReferences: meshv1.OwnerReferences(group),
		},
		Data: map[string]string{
			"config.yaml": buf.String(),
		},
	}, csum, nil
}

type traefikConfig struct {
	TCP tcpConfig `yaml:"tcp"`
	UDP udpConfig `yaml:"udp"`
}

type tcpConfig struct {
	Routers  map[string]routerConfig  `yaml:"routers"`
	Services map[string]serviceConfig `yaml:"services"`
}

type udpConfig struct {
	Routers  map[string]routerConfig  `yaml:"routers"`
	Services map[string]serviceConfig `yaml:"services"`
}

type routerConfig struct {
	EntryPoints []string  `yaml:"entryPoints"`
	Service     string    `yaml:"service"`
	Rule        string    `yaml:"rule,omitempty"`
	TLS         tlsConfig `yaml:"tls,omitempty"`
}

type serviceConfig struct {
	LoadBalancer loadBalancerConfig `yaml:"loadBalancer"`
}

type loadBalancerConfig struct {
	Servers []serverConfig `yaml:"servers"`
}

type serverConfig struct {
	Address string `yaml:"address"`
}

type tlsConfig struct {
	Passthrough bool `yaml:"passthrough"`
}
