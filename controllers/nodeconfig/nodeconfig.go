/*
Copyright 2023 Avi Zimmerman <avi.zimmerman@gmail.com>

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
	"crypto/sha256"
	"fmt"
	"sort"
	"time"

	"github.com/webmeshproj/webmesh/pkg/config"

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
	// WireGuardListenPort is the WireGuard listen port.
	WireGuardListenPort int
	// IsBootstrap is true if this is the bootstrap node group.
	IsBootstrap bool
	// BootstrapServers are the bootstrap servers.
	BootstrapServers map[string]string
	// BootstrapVoters are additional bootstrap voters.
	BootstrapVoters []string
	// JoinServer is the join server.
	JoinServer string
	// IsPersistent is true if this is a persistent node group.
	IsPersistent bool
	// CertDir is the cert directory.
	CertDir string
	// DetectEndpoints is true if endpoints should be detected.
	DetectEndpoints bool
	// AllowRemoteDetection is true if remote detection is allowed.
	AllowRemoteDetection bool
	// PersistentKeepalive is the persistent keepalive.
	PersistentKeepalive time.Duration
}

// Config represents a rendered node group config.
type Config struct {
	Options *config.Config
	raw     []byte
}

// Checksum returns the checksum of the config.
func (c *Config) Checksum() string {
	return fmt.Sprintf("%x", sha256.Sum256(c.raw))
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
	nodeopts := config.NewDefaultConfig("")

	// Global options
	nodeopts.Global.LogLevel = groupcfg.LogLevel
	nodeopts.Global.TLSCertFile = fmt.Sprintf(`%s/tls.crt`, opts.CertDir)
	nodeopts.Global.TLSKeyFile = fmt.Sprintf(`%s/tls.key`, opts.CertDir)
	nodeopts.Global.TLSCAFile = fmt.Sprintf(`%s/ca.crt`, opts.CertDir)
	nodeopts.Global.MTLS = true
	nodeopts.Global.VerifyChainOnly = mesh.Spec.Issuer.Create
	nodeopts.Global.DisableIPv6 = groupcfg.NoIPv6
	nodeopts.Global.DetectEndpoints = opts.DetectEndpoints
	nodeopts.Global.AllowRemoteDetection = opts.AllowRemoteDetection
	nodeopts.Global.DetectIPv6 = opts.DetectEndpoints // TODO: Make this a separate option

	// Endpoint and zone awareness options
	zoneAwarenessID := group.GetName()
	if id, ok := group.Labels[meshv1.ZoneAwarenessLabel]; ok {
		zoneAwarenessID = id
	}
	nodeopts.Mesh.ZoneAwarenessID = zoneAwarenessID
	nodeopts.Mesh.PrimaryEndpoint = opts.PrimaryEndpoint
	if len(opts.WireGuardEndpoints) > 0 {
		sort.Strings(opts.WireGuardEndpoints)
		nodeopts.WireGuard.Endpoints = opts.WireGuardEndpoints
	}

	// WireGuard options
	nodeopts.WireGuard.PersistentKeepAlive = opts.PersistentKeepalive
	nodeopts.WireGuard.ForceInterfaceName = true
	if opts.WireGuardListenPort > 0 {
		nodeopts.WireGuard.ListenPort = opts.WireGuardListenPort
	}

	// Bootstrap options
	if opts.IsBootstrap {
		nodeopts.Bootstrap.Enabled = true
		nodeopts.Bootstrap.Admin = meshv1.MeshAdminHostname(mesh)
		nodeopts.Bootstrap.IPv4Network = mesh.Spec.IPv4
		nodeopts.Bootstrap.DefaultNetworkPolicy = string(mesh.Spec.DefaultNetworkPolicy)
		nodeopts.Bootstrap.Transport.TCPAdvertiseAddress = opts.AdvertiseAddress
		nodeopts.Bootstrap.Transport.TCPServers = opts.BootstrapServers
		if len(opts.BootstrapVoters) > 0 {
			sort.Strings(opts.BootstrapVoters)
			nodeopts.Bootstrap.Voters = opts.BootstrapVoters
		}
	} else {
		if opts.JoinServer == "" {
			return nil, fmt.Errorf("join server is required for non bootstrap node groups")
		}
		nodeopts.Mesh.JoinAddress = opts.JoinServer
		nodeopts.Raft.RequestVote = groupcfg.Voter
	}

	// Storage options
	if opts.IsPersistent {
		nodeopts.Raft.DataDir = meshv1.DefaultDataDirectory
	} else {
		nodeopts.Raft.DataDir = ""
		nodeopts.Raft.InMemory = true
	}

	// Service options
	if groupcfg.Services != nil {
		nodeopts.Services.WebRTC.Enabled = groupcfg.Services.WebRTC != nil
		nodeopts.Services.API.MeshEnabled = groupcfg.Services.EnableMeshAPI
		nodeopts.Services.API.AdminEnabled = groupcfg.Services.EnableAdminAPI
		nodeopts.Services.MeshDNS.Enabled = groupcfg.Services.MeshDNS != nil
		nodeopts.Services.Metrics.Enabled = groupcfg.Services.Metrics != nil
		if groupcfg.Services.Metrics != nil {
			nodeopts.Services.Metrics.ListenAddress = groupcfg.Services.Metrics.ListenAddress
			nodeopts.Services.Metrics.Path = groupcfg.Services.Metrics.Path
		}
		if groupcfg.Services.WebRTC != nil {
			nodeopts.Services.WebRTC.STUNServers = groupcfg.Services.WebRTC.STUNServers
		}
		if groupcfg.Services.MeshDNS != nil {
			nodeopts.Services.MeshDNS.ListenUDP = groupcfg.Services.MeshDNS.ListenUDP
			nodeopts.Services.MeshDNS.ListenTCP = groupcfg.Services.MeshDNS.ListenTCP
		}
	}

	// Build the config
	out, err := nodeopts.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return &Config{
		Options: &nodeopts,
		raw:     out,
	}, nil
}
