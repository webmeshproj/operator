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
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/webmeshproj/node/pkg/nodecmd"

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
	Options *nodecmd.Options
	rawjson []byte
	raw     []byte
}

// Checksum returns the checksum of the config.
func (c *Config) Checksum() string {
	return fmt.Sprintf("%x", sha256.Sum256(c.rawjson))
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
	nodeopts.Global.LogLevel = groupcfg.LogLevel
	nodeopts.Global.TLSCertFile = fmt.Sprintf(`%s/tls.crt`, opts.CertDir)
	nodeopts.Global.TLSKeyFile = fmt.Sprintf(`%s/tls.key`, opts.CertDir)
	nodeopts.Global.TLSCAFile = fmt.Sprintf(`%s/ca.crt`, opts.CertDir)
	nodeopts.Global.MTLS = true
	nodeopts.Global.VerifyChainOnly = mesh.Spec.Issuer.Create
	nodeopts.Global.NoIPv6 = groupcfg.NoIPv6
	nodeopts.Global.DetectEndpoints = opts.DetectEndpoints
	nodeopts.Global.AllowRemoteDetection = opts.AllowRemoteDetection
	nodeopts.Global.DetectIPv6 = opts.DetectEndpoints // TODO: Make this a separate option

	// Endpoint and zone awareness options
	zoneAwarenessID := group.GetName()
	if id, ok := group.Labels[meshv1.ZoneAwarenessLabel]; ok {
		zoneAwarenessID = id
	}
	nodeopts.Mesh.Mesh.ZoneAwarenessID = zoneAwarenessID
	nodeopts.Mesh.Mesh.PrimaryEndpoint = opts.PrimaryEndpoint
	if len(opts.WireGuardEndpoints) > 0 {
		sort.Strings(opts.WireGuardEndpoints)
		nodeopts.Mesh.Mesh.WireGuardEndpoints = strings.Join(opts.WireGuardEndpoints, ",")
	}

	// WireGuard options
	nodeopts.Mesh.WireGuard.PersistentKeepAlive = opts.PersistentKeepalive
	nodeopts.Mesh.WireGuard.ForceInterfaceName = true
	if opts.WireGuardListenPort > 0 {
		nodeopts.Mesh.WireGuard.ListenPort = opts.WireGuardListenPort
	}

	// Bootstrap options
	if opts.IsBootstrap {
		nodeopts.Mesh.Bootstrap.Enabled = true
		nodeopts.Mesh.Bootstrap.Admin = meshv1.MeshAdminHostname(mesh)
		nodeopts.Mesh.Bootstrap.IPv4Network = mesh.Spec.IPv4
		nodeopts.Mesh.Bootstrap.DefaultNetworkPolicy = string(mesh.Spec.DefaultNetworkPolicy)
		nodeopts.Services.API.LeaderProxy = true
		nodeopts.Mesh.Bootstrap.AdvertiseAddress = opts.AdvertiseAddress
		if len(opts.BootstrapVoters) > 0 {
			sort.Strings(opts.BootstrapVoters)
			nodeopts.Mesh.Bootstrap.Voters = strings.Join(opts.BootstrapVoters, ",")
		}
		if len(opts.BootstrapServers) > 0 {
			var bootstrapServers sort.StringSlice
			for name, addr := range opts.BootstrapServers {
				bootstrapServers = append(bootstrapServers, fmt.Sprintf("%s=%s", name, addr))
			}
			sort.Sort(bootstrapServers)
			nodeopts.Mesh.Bootstrap.Servers = strings.Join(bootstrapServers, ",")
		}
	} else {
		if opts.JoinServer == "" {
			return nil, fmt.Errorf("join server is required for non bootstrap node groups")
		}
		nodeopts.Mesh.Mesh.JoinAddress = opts.JoinServer
		nodeopts.Mesh.Mesh.JoinAsVoter = groupcfg.Voter
		nodeopts.Mesh.Raft.LeaveOnShutdown = true // TODO: Make these separate options
	}

	// Storage options
	if opts.IsPersistent {
		nodeopts.Mesh.Raft.DataDir = meshv1.DefaultDataDirectory
	} else {
		nodeopts.Mesh.Raft.DataDir = ""
		nodeopts.Mesh.Raft.InMemory = true
	}

	// Service options
	if groupcfg.Services != nil {
		nodeopts.Services.API.LeaderProxy = opts.IsBootstrap || groupcfg.Services.EnableLeaderProxy
		nodeopts.Services.API.WebRTC = groupcfg.Services.WebRTC != nil
		nodeopts.Services.API.Mesh = groupcfg.Services.EnableMeshAPI
		nodeopts.Services.API.PeerDiscovery = groupcfg.Services.EnablePeerDiscoveryAPI
		nodeopts.Services.API.Admin = groupcfg.Services.EnableAdminAPI
		nodeopts.Services.MeshDNS.Enabled = groupcfg.Services.MeshDNS != nil
		nodeopts.Services.Metrics.Enabled = groupcfg.Services.Metrics != nil
		if groupcfg.Services.Metrics != nil {
			nodeopts.Services.Metrics.ListenAddress = groupcfg.Services.Metrics.ListenAddress
			nodeopts.Services.Metrics.Path = groupcfg.Services.Metrics.Path
		}
		if groupcfg.Services.WebRTC != nil {
			nodeopts.Services.API.STUNServers = strings.Join(groupcfg.Services.WebRTC.STUNServers, ",")
		}
		if groupcfg.Services.MeshDNS != nil {
			nodeopts.Services.MeshDNS.ListenUDP = groupcfg.Services.MeshDNS.ListenUDP
			nodeopts.Services.MeshDNS.ListenTCP = groupcfg.Services.MeshDNS.ListenTCP
		}
	}

	// Build the config
	var buf bytes.Buffer
	err := nodeopts.MarshalTo(&buf)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	j, err := json.Marshal(nodeopts)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return &Config{
		Options: nodeopts,
		rawjson: j,
		raw:     buf.Bytes(),
	}, nil
}
