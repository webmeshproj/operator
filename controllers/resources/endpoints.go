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
	"net"
	"net/netip"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	meshv1 "github.com/webmeshproj/operator/api/v1"
)

func NewNodeGroupPodEndpointSlices(mesh *meshv1.Mesh, group *meshv1.NodeGroup, pod *corev1.Pod, index int) ([]*discoveryv1.EndpointSlice, error) {
	var slices []*discoveryv1.EndpointSlice
	if len(pod.Status.PodIPs) == 0 {
		return nil, fmt.Errorf("pod IP is empty")
	}
	for _, ip := range pod.Status.PodIPs {
		if ip.IP == "" {
			return nil, fmt.Errorf("pod IP is empty")
		}
		addr, err := netip.ParseAddr(ip.IP)
		if err != nil {
			return nil, fmt.Errorf("parse pod IP: %w", err)
		}
		var addrType discoveryv1.AddressType
		var epName string
		if addr.Is4() {
			addrType = discoveryv1.AddressTypeIPv4
			epName = fmt.Sprintf("%s-%d-ipv4", meshv1.MeshNodeGroupLBName(mesh, group), index)
		} else {
			if addr.Is4In6() {
				// Convert to IPv4
				addr, _ = netip.AddrFromSlice(net.IP(addr.AsSlice()).To4())
				addrType = discoveryv1.AddressTypeIPv4
				epName = fmt.Sprintf("%s-%d-ipv4", meshv1.MeshNodeGroupLBName(mesh, group), index)
			} else {
				addrType = discoveryv1.AddressTypeIPv6
				epName = fmt.Sprintf("%s-%d-ipv6", meshv1.MeshNodeGroupLBName(mesh, group), index)
			}
		}
		ep := &discoveryv1.EndpointSlice{
			TypeMeta: metav1.TypeMeta{
				APIVersion: discoveryv1.SchemeGroupVersion.String(),
				Kind:       "EndpointSlice",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      epName,
				Namespace: group.GetNamespace(),
				Labels: map[string]string{
					"kubernetes.io/service-name":             meshv1.MeshNodeGroupLBName(mesh, group),
					"endpointslice.kubernetes.io/managed-by": "webmesh-operator",
				},
				OwnerReferences: meshv1.OwnerReferences(group),
			},
			AddressType: addrType,
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{addr.String()},
				},
			},
			Ports: []discoveryv1.EndpointPort{
				{
					Name:     Pointer("grpc"),
					Protocol: Pointer(corev1.ProtocolTCP),
					Port:     Pointer(int32(meshv1.DefaultGRPCPort)),
				},
				{
					Name:     Pointer(fmt.Sprintf("wireguard-%d", index)),
					Protocol: Pointer(corev1.ProtocolUDP),
					Port:     Pointer(int32(meshv1.DefaultWireGuardPort + index)),
				},
			},
		}
		slices = append(slices, ep)
	}
	return slices, nil
}
