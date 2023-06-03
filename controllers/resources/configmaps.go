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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	meshv1 "github.com/webmeshproj/operator/api/v1"
	"github.com/webmeshproj/operator/controllers/envoyconfig"
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

func NewNodeGroupEnvoyConfigMap(mesh *meshv1.Mesh, group *meshv1.NodeGroup) (*corev1.ConfigMap, string, error) {
	cfg, err := envoyconfig.New(envoyconfig.Options{
		Mesh:  mesh,
		Group: group,
	})
	if err != nil {
		return nil, "", err
	}
	annotations := group.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[meshv1.ConfigChecksumAnnotation] = cfg.Checksum()
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
			"envoy.yaml": string(cfg.Raw()),
		},
	}, cfg.Checksum(), nil
}
