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

// Package nodeconfig contains Webmesh node configuration rendering.
package nodeconfig

import (
	"fmt"
	"hash/crc32"
	"strings"

	"github.com/webmeshproj/node/pkg/global"
	"github.com/webmeshproj/node/pkg/nodecmd"
	"gopkg.in/yaml.v3"

	meshv1 "github.com/webmeshproj/operator/api/v1"
)

// Options are options for generating a node group config.
type Options struct {
	// Mesh is the mesh.
	Mesh *meshv1.Mesh
	// Group is the node group.
	Group *meshv1.NodeGroup
	// AdvertiseAddress is the advertise address.
	AdvertiseAddress string
	// PrimaryEndpoint is the primary endpoint.
	PrimaryEndpoint string
	// WireGuardEndpoints are the WireGuard endpoints.
	WireGuardEndpoints []string
	// IsBootstrap is true if this is the bootstrap node group.
	IsBootstrap bool
	// BootstrapServers are the bootstrap servers.
	BootstrapServers map[string]string
	// JoinServer is the join server.
	JoinServer string
	// IsPersistent is true if this is a persistent node group.
	IsPersistent bool
	// CertDir is the cert directory.
	CertDir string
}

// Config represents a rendered node group config.
type Config struct {
	Options *nodecmd.Options
	raw     []byte
}

// Checksum returns the checksum of the config.
func (c *Config) Checksum() string {
	return fmt.Sprintf("%x", crc32.ChecksumIEEE(c.raw))
}

// Raw returns the raw config.
func (c *Config) Raw() []byte {
	return c.raw
}

// New returns a new node group config.
func New(opts Options) (*Config, error) {
	group := opts.Group
	mesh := opts.Mesh

	// Merge config group if specified
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
	nodeopts := nodecmd.NewOptions()

	// Global options
	nodeopts.Global = &global.Options{
		LogLevel:        groupcfg.LogLevel,
		TLSCertFile:     fmt.Sprintf(`%s/tls.crt`, opts.CertDir),
		TLSKeyFile:      fmt.Sprintf(`%s/tls.key`, opts.CertDir),
		TLSCAFile:       fmt.Sprintf(`%s/ca.crt`, opts.CertDir),
		MTLS:            true,
		VerifyChainOnly: mesh.Spec.Issuer.Create,
		NoIPv6:          groupcfg.NoIPv6,
	}

	// Endpoint and zone awareness options
	nodeopts.Store.ZoneAwarenessID = group.GetName()
	nodeopts.Store.NodeEndpoint = opts.PrimaryEndpoint
	if len(opts.WireGuardEndpoints) > 0 {
		nodeopts.Store.NodeWireGuardEndpoints = strings.Join(opts.WireGuardEndpoints, ",")
	}

	// Bootstrap options
	if opts.IsBootstrap {
		nodeopts.Store.Bootstrap = true
		nodeopts.Store.BootstrapWithRaftACLs = true
		nodeopts.Store.Options.BootstrapIPv4Network = mesh.Spec.IPv4
		nodeopts.Services.EnableLeaderProxy = true
		nodeopts.Store.AdvertiseAddress = opts.AdvertiseAddress
		if len(opts.BootstrapServers) > 0 {
			var bootstrapServers []string
			for name, addr := range opts.BootstrapServers {
				bootstrapServers = append(bootstrapServers, fmt.Sprintf("%s=%s", name, addr))
			}
			nodeopts.Store.Options.BootstrapServers = strings.Join(bootstrapServers, ",")
		}
	} else {
		if opts.JoinServer == "" {
			return nil, fmt.Errorf("join server is required for non bootstrap node groups")
		}
		nodeopts.Store.Join = opts.JoinServer
	}

	// Storage options
	if opts.IsPersistent {
		nodeopts.Store.Options.DataDir = meshv1.DefaultDataDirectory
	} else {
		nodeopts.Store.Options.DataDir = ""
		nodeopts.Store.Options.InMemory = true
	}

	// Service options
	if groupcfg.Services != nil {
		nodeopts.Services.EnableLeaderProxy = opts.IsBootstrap || groupcfg.Services.EnableLeaderProxy
		nodeopts.Services.EnableMetrics = groupcfg.Services.Metrics != nil
		nodeopts.Services.EnableWebRTCAPI = groupcfg.Services.WebRTC != nil
		nodeopts.Services.EnableMeshDNS = groupcfg.Services.MeshDNS != nil
		nodeopts.Services.EnableMeshAPI = groupcfg.Services.EnableMeshAPI
		nodeopts.Services.EnablePeerDiscoveryAPI = groupcfg.Services.EnablePeerDiscoveryAPI
		if groupcfg.Services.Metrics != nil {
			nodeopts.Services.MetricsListenAddress = groupcfg.Services.Metrics.ListenAddress
			nodeopts.Services.MetricsPath = groupcfg.Services.Metrics.Path
		}
		if groupcfg.Services.WebRTC != nil {
			nodeopts.Services.STUNServers = strings.Join(groupcfg.Services.WebRTC.STUNServers, ",")
		}
		if groupcfg.Services.MeshDNS != nil {
			nodeopts.Services.MeshDNSListenUDP = groupcfg.Services.MeshDNS.ListenUDP
			nodeopts.Services.MeshDNSListenTCP = groupcfg.Services.MeshDNS.ListenTCP
		}
	}

	// Build the config
	out, err := yaml.Marshal(nodeopts)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return &Config{
		Options: nodeopts,
		raw:     out,
	}, nil
}
