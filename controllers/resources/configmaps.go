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
	"sigs.k8s.io/controller-runtime/pkg/client"

	meshv1 "github.com/webmeshproj/operator/api/v1"
)

// NodeGroupConfigOptions are options for generating a node group config.
type NodeGroupConfigOptions struct {
	// Mesh is the mesh.
	Mesh *meshv1.Mesh
	// Group is the node group.
	Group *meshv1.NodeGroup
	// OwnedBy is the object that owns the config.
	OwnedBy client.Object
	// IsBootstrap is true if this is the bootstrap node group.
	IsBootstrap bool
	// ExternalEndpoint is the external endpoint for the node group.
	ExternalEndpoint string
}

// NewNodeGroupConfigMap returns a new ConfigMap for a NodeGroup.
func NewNodeGroupConfigMap(opts NodeGroupConfigOptions) (cm *corev1.ConfigMap, csum string, err error) {
	group := opts.Group
	mesh := opts.Mesh

	// Merge config group if specified
	groupcfg := group.Spec.Config
	if group.Spec.ConfigGroup != "" {
		if mesh.Spec.ConfigGroups == nil {
			return nil, "", fmt.Errorf("config group %s not found", group.Spec.ConfigGroup)
		}
		configGroup, ok := mesh.Spec.ConfigGroups[group.Spec.ConfigGroup]
		if !ok {
			return nil, "", fmt.Errorf("config group %s not found", group.Spec.ConfigGroup)
		}
		groupcfg = configGroup.Merge(groupcfg)
	}
	nodeopts := nodecmd.NewOptions()

	// Global options
	nodeopts.Global = &global.Options{
		LogLevel:        groupcfg.LogLevel,
		TLSCertFile:     fmt.Sprintf(`%s/{{ env "POD_NAME" }}/tls.crt`, meshv1.DefaultTLSDirectory),
		TLSKeyFile:      fmt.Sprintf(`%s/{{ env "POD_NAME" }}/tls.key`, meshv1.DefaultTLSDirectory),
		TLSCAFile:       fmt.Sprintf(`%s/{{ env "POD_NAME" }}/ca.crt`, meshv1.DefaultTLSDirectory),
		MTLS:            true,
		VerifyChainOnly: mesh.Spec.Issuer.Create,
		NoIPv6:          groupcfg.NoIPv6,
	}

	// Endpoint and zone awareness options
	nodeopts.Store.ZoneAwarenessID = group.GetName()
	internalEndpoint := fmt.Sprintf(`{{ env "POD_NAME" }}.%s:%d`,
		meshv1.MeshNodeGroupHeadlessServiceFQDN(mesh, group), meshv1.DefaultWireGuardPort)
	wgendpoints := []string{internalEndpoint}
	if opts.ExternalEndpoint != "" {
		nodeopts.Store.NodeEndpoint = opts.ExternalEndpoint
		wgep := fmt.Sprintf(`%s:{{ add (intFile "%s/ordinal") %d }}`,
			opts.ExternalEndpoint, meshv1.DefaultDataDirectory, group.Spec.Service.WireGuardPort)
		wgendpoints = append(wgendpoints, wgep)
	} else if opts.IsBootstrap {
		// We need to at least use the internal endpoint for the bootstrap node group
		nodeopts.Store.NodeEndpoint = fmt.Sprintf(`{{ env "POD_NAME" }}.%s`,
			meshv1.MeshNodeGroupHeadlessServiceFQDN(mesh, group))
	}
	nodeopts.Store.NodeWireGuardEndpoints = strings.Join(wgendpoints, ",")

	// Bootstrap options
	if opts.IsBootstrap {
		nodeopts.Store.Bootstrap = true
		nodeopts.Store.BootstrapWithRaftACLs = true
		nodeopts.Store.Options.BootstrapIPv4Network = mesh.Spec.IPv4
		nodeopts.Services.EnableLeaderProxy = true
		if group.Spec.Replicas > 1 {
			nodeopts.Store.Options.AdvertiseAddress = fmt.Sprintf(`{{ env "POD_NAME" }}.%s:%d`,
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
			nodeopts.Store.Options.BootstrapServers = bootstrapServers.String()
		}
	}

	// Storage options
	if group.Spec.PVCSpec != nil {
		nodeopts.Store.Options.DataDir = meshv1.DefaultDataDirectory
	} else {
		nodeopts.Store.Options.DataDir = ""
		nodeopts.Store.Options.InMemory = true
	}

	// Build the config
	out, err := yaml.Marshal(nodeopts)
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
			Name:            meshv1.MeshNodeGroupConfigMapName(mesh, group),
			Namespace:       group.GetNamespace(),
			Labels:          meshv1.NodeGroupLabels(mesh, group),
			Annotations:     annotations,
			OwnerReferences: meshv1.OwnerReferences(opts.OwnedBy),
		},
		Data: map[string]string{
			"config.yaml": string(out),
		},
	}, csum, nil
}

func NewNodeGroupLBConfigMap(mesh *meshv1.Mesh, group *meshv1.NodeGroup, ownedBy client.Object) (cm *corev1.ConfigMap, csum string, err error) {
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
	for i := 0; i < int(group.Spec.Replicas); i++ {
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
			OwnerReferences: meshv1.OwnerReferences(ownedBy),
		},
		Data: map[string]string{
			"config.yaml": string(out),
		},
	}, csum, nil
}

func checksum(data []byte) string {
	return fmt.Sprintf("%x", crc32.ChecksumIEEE(data))
}
