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

// Package envoyconfig contains envoy load balancer configuration rendering.
package envoyconfig

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	meshv1 "github.com/webmeshproj/operator/api/v1"
)

// Options are options for generating an envoy config.
type Options struct {
	// Mesh is the mesh.
	Mesh *meshv1.Mesh
	// Group is the node group.
	Group *meshv1.NodeGroup
}

// Config is an envoy config.
type Config struct {
	raw     []byte
	rawjson []byte
}

// Checksum returns the checksum of the config.
func (c *Config) Checksum() string {
	return fmt.Sprintf("%x", sha256.Sum256(c.rawjson))
}

// Raw returns the raw config.
func (c *Config) Raw() []byte {
	return c.raw
}

// New creates a new envoy config.
func New(opts Options) (*Config, error) {
	conf := envoyConfig{
		Admin: envoyAdmin{
			Address: envoyAddress{
				SocketAddress: envoySocketAddress{
					Protocol:   "TCP",
					Address:    "::",
					PortValue:  9901,
					IPv4Compat: true,
				},
			},
		},
	}
	listeners := make([]envoyListener, int(*opts.Group.Spec.Replicas)+1)
	clusters := make([]envoyCluster, int(*opts.Group.Spec.Replicas)+1)
	listeners[0] = envoyListener{
		Name: "grpc",
		Address: envoyAddress{
			SocketAddress: envoySocketAddress{
				Protocol:   "TCP",
				Address:    "::",
				PortValue:  meshv1.DefaultGRPCPort,
				IPv4Compat: true,
			},
		},
		FilterChains: []envoyFilterChain{
			{
				Filters: []envoyFilter{
					{
						Name: "envoy.filters.network.tcp_proxy",
						TypedConfig: map[string]any{
							"@type":       "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy",
							"stat_prefix": fmt.Sprintf("grpc_%s", strings.Replace(opts.Group.GetName(), "-", "_", -1)),
							"cluster":     "grpc",
						},
					},
				},
			},
		},
	}
	clusters[0] = envoyCluster{
		Name:     "grpc",
		Type:     "STRICT_DNS",
		LBPolicy: "ROUND_ROBIN",
		LoadAssignment: envoyLoadAssignment{
			ClusterName: "grpc",
			Endpoints: []envoyLbEndpoint{
				{
					LbEndpoints: func() []envoyLbEndpointDetails {
						endpoints := make([]envoyLbEndpointDetails, int(*opts.Group.Spec.Replicas))
						for i := 0; i < int(*opts.Group.Spec.Replicas); i++ {
							endpoints[i] = envoyLbEndpointDetails{
								Endpoint: envoyEndpoint{
									Address: envoyAddress{
										SocketAddress: envoySocketAddress{
											Protocol:  "TCP",
											Address:   meshv1.MeshNodeClusterFQDN(opts.Mesh, opts.Group, i),
											PortValue: meshv1.DefaultGRPCPort,
										},
									},
								},
							}
						}
						return endpoints
					}(),
				},
			},
		},
	}
	for i := 0; i < int(*opts.Group.Spec.Replicas); i++ {
		name := fmt.Sprintf("node_%d", i)
		port := meshv1.DefaultWireGuardPort + i
		nodeAddr := meshv1.MeshNodeClusterFQDN(opts.Mesh, opts.Group, i)
		listener := envoyListener{
			Name: name,
			Address: envoyAddress{
				SocketAddress: envoySocketAddress{
					Protocol:   "UDP",
					Address:    "::",
					PortValue:  port,
					IPv4Compat: true,
				},
			},
			UDPListenerConfig: envoyUDPListenerConfig{
				DownstreamSocketConfig: envoyUDPDownstreamSocketConfig{
					MaxRxDatagramSize: 9000,
				},
			},
			ListenerFilters: []envoyListenerFilter{
				{
					Name: "envoy.filters.udp_listener.udp_proxy",
					TypedConfig: map[string]any{
						"@type":       "type.googleapis.com/envoy.extensions.filters.udp.udp_proxy.v3.UdpProxyConfig",
						"stat_prefix": name,
						"matcher": map[string]any{
							"on_no_match": map[string]any{
								"action": map[string]any{
									"name": "route",
									"typed_config": map[string]any{
										"@type":   "type.googleapis.com/envoy.extensions.filters.udp.udp_proxy.v3.Route",
										"cluster": name,
									},
								},
							},
						},
						"upstream_socket_config": map[string]any{
							"max_rx_datagram_size": 9000,
						},
					},
				},
			},
		}
		cluster := envoyCluster{
			Name:     name,
			Type:     "LOGICAL_DNS",
			LBPolicy: "ROUND_ROBIN",
			LoadAssignment: envoyLoadAssignment{
				ClusterName: name,
				Endpoints: []envoyLbEndpoint{
					{
						LbEndpoints: []envoyLbEndpointDetails{
							{
								Endpoint: envoyEndpoint{
									Address: envoyAddress{
										SocketAddress: envoySocketAddress{
											Protocol:  "UDP",
											Address:   nodeAddr,
											PortValue: meshv1.DefaultWireGuardPort + i,
										},
									},
								},
							},
						},
					},
				},
			},
		}
		listeners[i+1] = listener
		clusters[i+1] = cluster
	}
	conf.StaticResources.Listeners = listeners
	conf.StaticResources.Clusters = clusters

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	err := enc.Encode(&conf)
	if err != nil {
		return nil, err
	}
	rawjson, err := json.Marshal(conf)
	if err != nil {
		return nil, err
	}
	return &Config{
		raw:     buf.Bytes(),
		rawjson: rawjson,
	}, nil
}

type envoyConfig struct {
	Admin           envoyAdmin           `yaml:"admin"`
	StaticResources envoyStaticResources `yaml:"static_resources"`
}

type envoyAdmin struct {
	Address envoyAddress `yaml:"address"`
}

type envoyStaticResources struct {
	Listeners []envoyListener `yaml:"listeners"`
	Clusters  []envoyCluster  `yaml:"clusters"`
}

type envoyListener struct {
	Name              string                 `yaml:"name"`
	Address           envoyAddress           `yaml:"address"`
	UDPListenerConfig envoyUDPListenerConfig `yaml:"udp_listener_config,omitempty"`
	ListenerFilters   []envoyListenerFilter  `yaml:"listener_filters,omitempty"`
	FilterChains      []envoyFilterChain     `yaml:"filter_chains,omitempty"`
}

type envoyListenerFilter struct {
	Name        string         `yaml:"name"`
	TypedConfig map[string]any `yaml:"typed_config"`
}

type envoyFilterChain struct {
	Filters []envoyFilter `yaml:"filters"`
}

type envoyFilter struct {
	Name        string         `yaml:"name"`
	TypedConfig map[string]any `yaml:"typed_config"`
}

type envoyCluster struct {
	Name           string              `yaml:"name"`
	Type           string              `yaml:"type"`
	LBPolicy       string              `yaml:"lb_policy"`
	LoadAssignment envoyLoadAssignment `yaml:"load_assignment"`
}

type envoyUDPListenerConfig struct {
	DownstreamSocketConfig envoyUDPDownstreamSocketConfig `yaml:"downstream_socket_config"`
}

type envoyUDPDownstreamSocketConfig struct {
	MaxRxDatagramSize int `yaml:"max_rx_datagram_size"`
}

type envoyLoadAssignment struct {
	ClusterName string            `yaml:"cluster_name"`
	Endpoints   []envoyLbEndpoint `yaml:"endpoints"`
}

type envoyLbEndpoint struct {
	LbEndpoints []envoyLbEndpointDetails `yaml:"lb_endpoints"`
}

type envoyLbEndpointDetails struct {
	Endpoint envoyEndpoint `yaml:"endpoint"`
}

type envoyEndpoint struct {
	Address envoyAddress `yaml:"address"`
}

type envoyAddress struct {
	SocketAddress envoySocketAddress `yaml:"socket_address"`
}

type envoySocketAddress struct {
	Protocol   string `yaml:"protocol"`
	Address    string `yaml:"address"`
	PortValue  int    `yaml:"port_value"`
	IPv4Compat bool   `yaml:"ipv4_compat,omitempty"`
}
