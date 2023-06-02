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

package v1

const (
	// DefaultNodeImage is the default image to use for nodes.
	DefaultNodeImage = "ghcr.io/webmeshproj/node:latest"
	// DefaultNodeLBImage is the default image to use for node load balancers.
	DefaultNodeLBImage = "traefik:v3.0"
	// DefaultRaftPort is the default port to use for Raft.
	DefaultRaftPort = 9443
	// DefaultGRPCPort is the default port to use for gRPC.
	DefaultGRPCPort = 8443
	// DefaultWireGuardPort is the default port to use for WireGuard.
	DefaultWireGuardPort = 51820
	// DefaultStorageSize is the default storage size to use for nodes.
	DefaultStorageSize = "1Gi"
	// DefaultDataDirectory is the default data directory to use for nodes.
	DefaultDataDirectory = "/data"
	// DefaultTLSDirectory is the default TLS directory to use for nodes.
	DefaultTLSDirectory = "/etc/webmesh/tls"
	// FieldOwner is the field owner to use for all resources.
	FieldOwner = "webmesh-operator"
	// MeshNameLabel is the label to use for the Mesh name.
	MeshNameLabel = "webmesh.io/mesh-name"
	// MeshNamespaceLabel is the label to use for the Mesh namespace.
	MeshNamespaceLabel = "webmesh.io/mesh-namespace"
	// NodeGroupNameLabel is the label to use for the NodeGroup name.
	NodeGroupNameLabel = "webmesh.io/nodegroup-name"
	// NodeGroupNamespaceLabel is the label to use for the NodeGroup namespace.
	NodeGroupNamespaceLabel = "webmesh.io/nodegroup-namespace"
	// NodeGroupLBLabel is the label to use for the NodeGroup load balancer.
	NodeGroupLBLabel = "webmesh.io/nodegroup-lb"
	// ConfigChecksumAnnotation is the annotation to use for configmap checksums.
	ConfigChecksumAnnotation = "webmesh.io/config-checksum"
	// BootstrapNodeGroupAnnotation is the annotation to use for bootstrap node groups.
	// This should only be set by the controller for bootstrap node groups. It is also
	// used as a label selector for bootstrap node groups.
	BootstrapNodeGroupAnnotation = "webmesh.io/bootstrap-nodegroup"
)
