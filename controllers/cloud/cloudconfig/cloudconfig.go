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

// Package cloudconfig contains Webmesh node cloud config rendering.
// Returned cloud-configs are intended for use with container-optimized
// OSes.
package cloudconfig

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/webmeshproj/operator/controllers/nodeconfig"
)

// Config represents a rendered cloud config.
type Config struct {
	// Raw is the raw cloud config.
	raw []byte
}

// Checksum returns the checksum of the config.
func (c *Config) Checksum() string {
	return fmt.Sprintf("%x", crc32.ChecksumIEEE(c.raw))
}

// Raw returns the raw config.
func (c *Config) Raw() []byte {
	return c.raw
}

// Options are options for generating a cloud config.
type Options struct {
	// Image is the image to run.
	Image string
	// Config is the node config.
	Config *nodeconfig.Config
	// TLSCert is the TLS cert.
	TLSCert []byte
	// TLSKey is the TLS key.
	TLSKey []byte
	// CA is the CA.
	CA []byte
}

// New returns a new cloud config.
func New(opts *Options) (*Config, error) {
	out := cloudConfig{
		WriteFiles: []writeFile{
			{
				Path:        "/etc/systemd/system/config-firewall.service",
				Permissions: "0644",
				Owner:       "root",
				Content:     configFirewallUnit,
			},
			{
				Path:        "/etc/systemd/system/node.service",
				Permissions: "0644",
				Owner:       "root",
				Content:     nodeContainerUnit(opts.Image),
			},
			{
				Path:        "/etc/webmesh/config.yaml",
				Permissions: "0644",
				Owner:       "root",
				Content:     string(opts.Config.Raw()),
			},
			{
				Path:        "/etc/webmesh/tls.crt",
				Permissions: "0644",
				Owner:       "root",
				Content:     string(opts.TLSCert),
			},
			{
				Path:        "/etc/webmesh/tls.key",
				Permissions: "0644",
				Owner:       "root",
				Content:     string(opts.TLSKey),
			},
			{
				Path:        "/etc/webmesh/ca.crt",
				Permissions: "0644",
				Owner:       "root",
				Content:     string(opts.CA),
			},
		},
		RunCmd: []string{
			"systemctl daemon-reload",
			"systemctl enable docker",
			"systemctl start docker",
			"systemctl start node",
		},
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return nil, err
	}
	return &Config{
		raw: append([]byte("#cloud-config\n\n"), data...),
	}, nil
}

type cloudConfig struct {
	WriteFiles []writeFile `yaml:"write_files"`
	Packages   []string    `yaml:"packages"`
	RunCmd     []string    `yaml:"runcmd"`
}

type writeFile struct {
	Path        string `yaml:"path"`
	Permissions string `yaml:"permissions"`
	Owner       string `yaml:"owner"`
	Content     string `yaml:"content"`
}

func nodeContainerUnit(image string) string {
	var buf bytes.Buffer
	_ = nodeContainerUnitTemplate.Execute(&buf, struct {
		Image string
	}{
		Image: image,
	})
	return buf.String()
}

var configFirewallUnit = `[Unit]
Description=Configures the host firewall

[Service]
Type=oneshot
RemainAfterExit=true
ExecStart=/sbin/iptables -A INPUT -j ACCEPT`

var nodeContainerUnitTemplate = template.Must(template.New("nodecontainer").Parse(`[Unit]
Description=node
After=docker.service config-firewall.service
Wants=docker.service config-firewall.service

[Service]
ExecStart=/usr/bin/docker run --rm \
  --pull always \
  --name node \
  --network host \
  --privileged \
  --cap-add NET_ADMIN \
  --cap-add NET_RAW \
  --cap-add SYS_MODULE \
  -v /lib/modules:/lib/modules \
  -v /dev/net/tun:/dev/net/tun \
  -v /etc/webmesh:/etc/webmesh \
  {{ .Image }}
ExecStop=/usr/bin/docker kill node
Restart=always

[Install]
WantedBy=multi-user.target
`))
