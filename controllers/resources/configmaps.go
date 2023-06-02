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
	"hash/crc32"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	meshv1 "github.com/webmeshproj/operator/api/v1"
	"github.com/webmeshproj/operator/controllers/nodeconfig"
)

// NewNodeGroupConfigMap returns a new ConfigMap for a NodeGroup.
func NewNodeGroupConfigMap(mesh *meshv1.Mesh, group *meshv1.NodeGroup, conf *nodeconfig.Config) (cm *corev1.ConfigMap) {
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
			Name:            meshv1.MeshNodeGroupConfigMapName(mesh, group),
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

func NewNodeGroupLBConfigMap(mesh *meshv1.Mesh, group *meshv1.NodeGroup) (cm *corev1.ConfigMap, csum string, err error) {
	cfg := map[string]map[string]any{
		"tcp": make(map[string]any),
		"udp": make(map[string]any),
	}
	cfg["tcp"]["routers"] = map[string]any{
		"grpc": map[string]any{
			"entryPoints": []string{"grpc"},
			"rule":        "HostSNI(`*`)",
			"service":     "grpc",
			"tls":         map[string]any{"passthrough": true},
		},
	}
	cfg["tcp"]["services"] = map[string]any{
		"grpc": map[string]any{
			"loadBalancer": map[string]any{
				"servers": []map[string]any{
					{"address": fmt.Sprintf(`%s:%d`,
						meshv1.MeshNodeGroupHeadlessServiceFQDN(mesh, group), meshv1.DefaultGRPCPort)},
				},
			},
		},
	}
	udprouters := make(map[string]any)
	udpservices := make(map[string]any)
	for i := 0; i < int(*group.Spec.Cluster.Replicas); i++ {
		udprouters[fmt.Sprintf("wg%d", i)] = map[string]any{
			"entryPoints": []string{fmt.Sprintf("wg%d", i)},
			"service":     fmt.Sprintf("wg%d", i),
		}
		udpservices[fmt.Sprintf("wg%d", i)] = map[string]any{
			"loadBalancer": map[string]any{
				"servers": []map[string]any{
					{"address": fmt.Sprintf(`%s:%d`,
						meshv1.MeshNodeClusterFQDN(mesh, group, i), meshv1.DefaultWireGuardPort)},
				},
			},
		}

	}
	cfg["udp"]["routers"] = udprouters
	cfg["udp"]["services"] = udpservices
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("marshal config: %w", err)
	}
	annotations := group.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	csum = checksum(out)
	annotations[meshv1.ConfigChecksumAnnotation] = csum
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            meshv1.MeshNodeGroupLBName(mesh, group),
			Namespace:       group.GetNamespace(),
			Labels:          meshv1.NodeGroupLBLabels(mesh, group),
			Annotations:     annotations,
			OwnerReferences: meshv1.OwnerReferences(group),
		},
		Data: map[string]string{
			"config.yaml": string(out),
		},
	}, csum, nil
}

func checksum(data []byte) string {
	return fmt.Sprintf("%x", crc32.ChecksumIEEE(data))
}
