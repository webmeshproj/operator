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

// NodeGroupConfig defines the desired Webmesh configurations for a group of nodes.
type NodeGroupConfig struct {
	// LogLevel is the log level to use for the node containers in this
	// group.
	// +kubebuilder:Validation:Enum:=debug;info;warn;error
	// +kubebuilder:default:="info"
	// +optional
	LogLevel string `json:"logLevel,omitempty"`

	// NoIPv6 is true if IPv6 should not be used for the node group.
	// +optional
	NoIPv6 bool `json:"noIPv6,omitempty"`

	// Services is the configuration for services enabled for this group.
	// +optional
	Services *NodeServicesConfig `json:"services,omitempty"`
}

// Merge merges the given NodeGroupConfig into this NodeGroupConfig. The
// given NodeGroupConfig takes precedence. The merged NodeGroupConfig is
// returned for convenience. If both are nil, a default NodeGroupConfig is
// returned.
func (c *NodeGroupConfig) Merge(in *NodeGroupConfig) *NodeGroupConfig {
	if in == nil && c == nil {
		var empty NodeGroupConfig
		empty.Default()
		return &empty
	}
	if in == nil {
		return c
	}
	if c == nil {
		return in
	}
	if in.LogLevel != "" {
		c.LogLevel = in.LogLevel
	}
	if in.NoIPv6 {
		c.NoIPv6 = true
	}
	if in.Services != nil {
		if c.Services == nil {
			c.Services = &NodeServicesConfig{}
		}
		c.Services = c.Services.Merge(in.Services)
	}
	return c
}

// Default sets default values for any unset fields.
func (c *NodeGroupConfig) Default() {
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.Services != nil {
		c.Services.Default()
	}
}

// NodeServicesConfig defines the configurations for the services enabled
// on a group of nodes.
type NodeServicesConfig struct {
	// Metrics is the configuration for metrics enabled for this group.
	// +optional
	Metrics *NodeMetricsConfig `json:"metrics,omitempty"`

	// WebRTC is the configuration for WebRTC enabled for this group.
	// +optional
	WebRTC *NodeWebRTCConfig `json:"webRTC,omitempty"`

	// MeshDNS is the configuration for MeshDNS enabled for this group.
	// +optional
	MeshDNS *NodeMeshDNSConfig `json:"meshDNS,omitempty"`

	// EnableLeaderProxy is true if leader proxy should be enabled for
	// this group.
	// +optional
	EnableLeaderProxy bool `json:"enableLeaderProxy,omitempty"`

	// EnableMeshAPI is true if the Mesh API should be enabled for this
	// group.
	// +optional
	EnableMeshAPI bool `json:"enableMeshAPI,omitempty"`

	// EnablePeerDiscoveryAPI is true if peer discovery API should be enabled for
	// this group.
	// +optional
	EnablePeerDiscoveryAPI bool `json:"enablePeerDiscoveryAPI,omitempty"`
}

// Merge merges the given NodeServicesConfig into this NodeServicesConfig. The
// given NodeServicesConfig takes precedence. The merged NodeServicesConfig is
// returned for convenience. If both are nil, a default NodeServicesConfig is
// returned.
func (c *NodeServicesConfig) Merge(in *NodeServicesConfig) *NodeServicesConfig {
	if in == nil && c == nil {
		var empty NodeServicesConfig
		empty.Default()
		return &empty
	}
	if in == nil {
		return c
	}
	if c == nil {
		return in
	}
	if in.Metrics != nil {
		c.Metrics = c.Metrics.Merge(in.Metrics)
	}
	if in.WebRTC != nil {
		c.WebRTC = c.WebRTC.Merge(in.WebRTC)
	}
	if in.MeshDNS != nil {
		c.MeshDNS = c.MeshDNS.Merge(in.MeshDNS)
	}
	if in.EnableLeaderProxy {
		c.EnableLeaderProxy = true
	}
	if in.EnableMeshAPI {
		c.EnableMeshAPI = true
	}
	if in.EnablePeerDiscoveryAPI {
		c.EnablePeerDiscoveryAPI = true
	}
	return c
}

// Default sets default values for any unset fields.
func (c *NodeServicesConfig) Default() {
	if c.Metrics != nil {
		c.Metrics.Default()
	}
	if c.WebRTC != nil {
		c.WebRTC.Default()
	}
	if c.MeshDNS != nil {
		c.MeshDNS.Default()
	}
}

// NodeMetricsConfig defines the configurations for metrics enabled
// for a group of nodes.
type NodeMetricsConfig struct {
	// ListenAddress is the address to listen on for metrics.
	// +kubebuilder:default:=":8080"
	// +optional
	ListenAddress string `json:"listenAddress,omitempty"`

	// Path is the path to expose metrics on.
	// +kubebuilder:default:="/metrics"
	// +optional
	Path string `json:"path,omitempty"`
}

// Merge merges the given NodeMetricsConfig into this NodeMetricsConfig. The
// given NodeMetricsConfig takes precedence. The merged NodeMetricsConfig is
// returned for convenience. If both are nil, a default NodeMetricsConfig is
// returned.
func (c *NodeMetricsConfig) Merge(in *NodeMetricsConfig) *NodeMetricsConfig {
	if in == nil && c == nil {
		var empty NodeMetricsConfig
		empty.Default()
		return &empty
	}
	if in == nil {
		return c
	}
	if c == nil {
		return in
	}
	if in.ListenAddress != "" {
		c.ListenAddress = in.ListenAddress
	}
	if in.Path != "" {
		c.Path = in.Path
	}
	return c
}

// Default sets default values for any unset fields.
func (c *NodeMetricsConfig) Default() {
	if c.ListenAddress == "" {
		c.ListenAddress = ":8080"
	}
	if c.Path == "" {
		c.Path = "/metrics"
	}
}

// NodeWebRTCConfig defines the desired WebRTC configurations for a group of nodes.
type NodeWebRTCConfig struct {
	// STUNServers is the list of STUN servers to use for WebRTC.
	// +kubebuilder:default:={"stun:stun.l.google.com:19302"}
	// +optional
	STUNServers []string `json:"stunServers,omitempty"`
}

// Merge merges the given NodeWebRTCConfig into this NodeWebRTCConfig. The
// given NodeWebRTCConfig takes precedence. The merged NodeWebRTCConfig is
// returned for convenience. If both are nil, a default NodeWebRTCConfig is
// returned.
func (c *NodeWebRTCConfig) Merge(in *NodeWebRTCConfig) *NodeWebRTCConfig {
	if in == nil && c == nil {
		var empty NodeWebRTCConfig
		empty.Default()
		return &empty
	}
	if in == nil {
		return c
	}
	if c == nil {
		return in
	}
	if len(in.STUNServers) > 0 {
		c.STUNServers = in.STUNServers
	}
	return c
}

// Default sets default values for any unset fields.
func (c *NodeWebRTCConfig) Default() {
	if len(c.STUNServers) == 0 {
		c.STUNServers = []string{"stun:stun.l.google.com:19302"}
	}
}

// NodeMeshDNSConfig defines the desired MeshDNS configurations for a group of nodes.
type NodeMeshDNSConfig struct {
	// ListenUDP is the address to listen on for MeshDNS UDP.
	// +kubebuilder:default:=":5353"
	// +optional
	ListenUDP string `json:"listenUDP,omitempty"`

	// ListenTCP is the address to listen on for MeshDNS TCP.
	// +kubebuilder:default:=":5353"
	// +optional
	ListenTCP string `json:"listenTCP,omitempty"`
}

// Merge merges the given NodeMeshDNSConfig into this NodeMeshDNSConfig. The
// given NodeMeshDNSConfig takes precedence. The merged NodeMeshDNSConfig is
// returned for convenience. If both are nil, a default NodeMeshDNSConfig is
// returned.
func (c *NodeMeshDNSConfig) Merge(in *NodeMeshDNSConfig) *NodeMeshDNSConfig {
	if in == nil && c == nil {
		var empty NodeMeshDNSConfig
		empty.Default()
		return &empty
	}
	if in == nil {
		return c
	}
	if c == nil {
		return in
	}
	if in.ListenUDP != "" {
		c.ListenUDP = in.ListenUDP
	}
	if in.ListenTCP != "" {
		c.ListenTCP = in.ListenTCP
	}
	return c
}

// Default sets default values for any unset fields.
func (c *NodeMeshDNSConfig) Default() {
	if c.ListenUDP == "" {
		c.ListenUDP = ":5353"
	}
	if c.ListenTCP == "" {
		c.ListenTCP = ":5353"
	}
}
