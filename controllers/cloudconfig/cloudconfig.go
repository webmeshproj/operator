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

// Package cloudconfig contains Webmesh node cloud config rendering.
// Returned cloud-configs are intended for use with ubuntu images.
package cloudconfig

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"text/template"

	"gopkg.in/yaml.v3"

	meshv1 "github.com/webmeshproj/operator/api/v1"
	"github.com/webmeshproj/operator/controllers/nodeconfig"
)

// Config represents a rendered cloud config.
type Config struct {
	// Raw is the raw cloud config.
	raw []byte
}

// Checksum returns the checksum of the config.
func (c *Config) Checksum() string {
	return fmt.Sprintf("%x", sha256.Sum256(c.raw))
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
func New(opts Options) (*Config, error) {
	out := cloudConfig{
		WriteFiles: []writeFile{
			{
				Path:        "/etc/docker/daemon.json",
				Permissions: "0644",
				Owner:       "root",
				// TODO: Ensure this is compatible with the mesh network and VPC
				Content: `{"bip": "192.168.254.1/24"}`,
			},
			{
				Path:        "/etc/systemd/system/node.service",
				Permissions: "0644",
				Owner:       "root",
				Content:     nodeContainerUnit(&opts),
			},
			{
				Path:        "/etc/webmesh/config.yaml",
				Permissions: "0644",
				Owner:       "root",
				Content:     string(opts.Config.Raw()),
			},
			{
				Path:        fmt.Sprintf("%s/tls.crt", meshv1.DefaultTLSDirectory),
				Permissions: "0644",
				Owner:       "root",
				Content:     string(opts.TLSCert),
			},
			{
				Path:        fmt.Sprintf("%s/tls.key", meshv1.DefaultTLSDirectory),
				Permissions: "0644",
				Owner:       "root",
				Content:     string(opts.TLSKey),
			},
			{
				Path:        fmt.Sprintf("%s/ca.crt", meshv1.DefaultTLSDirectory),
				Permissions: "0644",
				Owner:       "root",
				Content:     string(opts.CA),
			},
		},
		Packages: []string{
			"apt-transport-https",
			"ca-certificates",
			"curl",
			"gnupg",
			"lsb-release",
			"unattended-upgrades",
			"wireguard-tools",
			"net-tools",
		},
		RunCmd: []string{
			"sysctl -w net.ipv4.conf.all.forwarding=1",
			"sysctl -w net.ipv6.conf.all.forwarding=1",
			"mkdir -p /etc/apt/keyrings",
			"curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg",
			`echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null`,
			"apt-get update",
			"apt-get install -y docker-ce docker-ce-cli containerd.io",
			"mkdir -p /var/lib/webmesh/data",
			"systemctl daemon-reload",
			"systemctl enable docker",
			"systemctl start docker",
			"systemctl start node",
		},
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	err := enc.Encode(out)
	if err != nil {
		return nil, err
	}
	return &Config{
		raw: append([]byte("#cloud-config\n\n"), buf.Bytes()...),
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

func nodeContainerUnit(opts *Options) string {
	var buf bytes.Buffer
	_ = nodeContainerUnitTemplate.Execute(&buf, struct {
		Image   string
		DataDir string
	}{
		Image:   opts.Image,
		DataDir: opts.Config.Options.Mesh.Raft.DataDir,
	})
	return buf.String()
}

var nodeContainerUnitTemplate = template.Must(template.New("nodecontainer").Parse(`[Unit]
Description=node
After=docker.service
Wants=docker.service

[Service]
ExecStartPre=-/usr/sbin/nft flush ruleset
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
  -v /var/lib/webmesh/data:{{ .DataDir }} \
  {{ .Image }} --config /etc/webmesh/config.yaml
ExecStop=/usr/bin/docker kill node
Restart=always

[Install]
WantedBy=multi-user.target
`))
