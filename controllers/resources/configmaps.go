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
	"strings"

	"github.com/webmeshproj/node/pkg/global"
	"github.com/webmeshproj/node/pkg/nodecmd"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	meshv1 "github.com/webmeshproj/operator/api/v1"
)

// NewNodeGroupConfigMap returns a new ConfigMap for a NodeGroup.
func NewNodeGroupConfigMap(mesh *meshv1.Mesh, group *meshv1.NodeGroup, isBootstrap bool) (*corev1.ConfigMap, error) {
	groupcfg := group.Spec.Config
	if group.Spec.ConfigGroup != "" {
		if mesh.Spec.ConfigGroups == nil {
			return nil, fmt.Errorf("config group %s not found", group.Spec.ConfigGroup)
		}
		configGroup, ok := mesh.Spec.ConfigGroups[group.Spec.ConfigGroup]
		if !ok {
			return nil, fmt.Errorf("config group %s not found", group.Spec.ConfigGroup)
		}
		groupcfg = configGroup.Merge(groupcfg)
	}
	opts := nodecmd.NewOptions()
	opts.Global = &global.Options{
		LogLevel:        groupcfg.LogLevel,
		TLSCertFile:     fmt.Sprintf(`%s/{{ env "POD_NAME" }}/tls.crt`, meshv1.DefaultTLSDirectory),
		TLSKeyFile:      fmt.Sprintf(`%s/{{ env "POD_NAME" }}/tls.key`, meshv1.DefaultTLSDirectory),
		TLSCAFile:       fmt.Sprintf(`%s/{{ env "POD_NAME" }}/ca.crt`, meshv1.DefaultTLSDirectory),
		MTLS:            true,
		VerifyChainOnly: mesh.Spec.Issuer.Create,
		NoIPv6:          groupcfg.NoIPv6,
	}
	// TODO: Technically only when we are exposing the node group as a service.
	// Also need to support the endpoints from any load balancers created.
	opts.Store.NodeEndpoint = fmt.Sprintf(`{{ env "POD_NAME" }}.%s`,
		meshv1.MeshNodeGroupHeadlessServiceFQDN(mesh, group))
	opts.Store.NodeWireGuardEndpoints = fmt.Sprintf(`{{ env "POD_NAME" }}.%s:%d`,
		meshv1.MeshNodeGroupHeadlessServiceFQDN(mesh, group), meshv1.DefaultWireGuardPort)
	// TODO: make this configurable
	opts.Store.ZoneAwarenessID = group.GetName()
	if isBootstrap {
		opts.Store.Bootstrap = true
		opts.Store.BootstrapWithRaftACLs = true
		opts.Store.Options.BootstrapIPv4Network = mesh.Spec.IPv4
		opts.Services.EnableLeaderProxy = true
		if group.Spec.Replicas > 1 {
			opts.Store.Options.AdvertiseAddress = fmt.Sprintf(`{{ env "POD_NAME" }}.%s:%d`,
				meshv1.MeshNodeGroupHeadlessServiceFQDN(mesh, group), meshv1.DefaultRaftPort)
			var bootstrapServers strings.Builder
			for i := 0; i < int(group.Spec.Replicas); i++ {
				bootstrapServers.WriteString(fmt.Sprintf("%s=%s:%d",
					meshv1.MeshNodeHostname(mesh, group, i),
					meshv1.MeshNodeClusterFQDN(mesh, group, i),
					meshv1.DefaultRaftPort,
				))
				if i < int(group.Spec.Replicas)-1 {
					bootstrapServers.WriteString(",")
				}
			}
			opts.Store.Options.BootstrapServers = bootstrapServers.String()
		}
	}
	if group.Spec.PVCSpec != nil {
		opts.Store.Options.DataDir = meshv1.DefaultDataDirectory
	} else {
		opts.Store.Options.DataDir = ""
		opts.Store.Options.InMemory = true
	}
	out, err := yaml.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	annotations := group.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[meshv1.NodeGroupConfigChecksumAnnotation] = checksum(out)
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            meshv1.MeshNodeGroupConfigMapName(mesh, group),
			Namespace:       group.GetNamespace(),
			Labels:          meshv1.NodeGroupLabels(mesh, group),
			Annotations:     group.GetAnnotations(),
			OwnerReferences: meshv1.OwnerReferences(mesh),
		},
		Data: map[string]string{
			"config.yaml": string(out),
		},
	}, nil
}

func checksum(data []byte) string {
	out := crc32.ChecksumIEEE(data)
	return fmt.Sprintf("%x", out)
}
