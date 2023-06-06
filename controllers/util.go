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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	meshv1 "github.com/webmeshproj/operator/api/v1"
)

var ErrLBNotReady = errors.New("load balancer not ready")

func getLBExternalIPs(ctx context.Context, cli client.Client, mesh *meshv1.Mesh, group *meshv1.NodeGroup) ([]string, error) {
	var lbService corev1.Service
	err := cli.Get(ctx, client.ObjectKey{
		Name:      meshv1.MeshNodeGroupLBName(mesh, group),
		Namespace: mesh.GetNamespace(),
	}, &lbService)
	if err != nil {
		return nil, fmt.Errorf("fetch load balancer service: %w", err)
	}
	var externalIPs []string
	switch lbService.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		if len(lbService.Status.LoadBalancer.Ingress) == 0 {
			return nil, ErrLBNotReady
		}
		for _, ingress := range lbService.Status.LoadBalancer.Ingress {
			if ingress.IP == "" {
				return nil, ErrLBNotReady
			}
			externalIPs = append(externalIPs, ingress.IP)
		}
		for _, ip := range lbService.Spec.ClusterIPs {
			addr, err := netip.ParseAddr(ip)
			if err != nil {
				return nil, fmt.Errorf("parse cluster IP: %w", err)
			}
			if !addr.IsPrivate() {
				externalIPs = append(externalIPs, addr.String())
			}
		}
	case corev1.ServiceTypeNodePort:
		return nil, fmt.Errorf("node port not supported")
	case corev1.ServiceTypeClusterIP:
		for _, ip := range lbService.Spec.ClusterIPs {
			addr, err := netip.ParseAddr(ip)
			if err != nil {
				return nil, fmt.Errorf("parse cluster IP: %w", err)
			}
			if !addr.IsPrivate() {
				externalIPs = append(externalIPs, addr.String())
			}
		}
		clusterIP, err := netip.ParseAddr(lbService.Spec.ClusterIP)
		if err != nil {
			return nil, fmt.Errorf("parse cluster IP: %w", err)
		}
		if !clusterIP.IsPrivate() {
			externalIPs = append(externalIPs, clusterIP.String())
		}
	default:
		return nil, fmt.Errorf("service has unknown type: %s", lbService.Spec.Type)
	}
	if len(externalIPs) == 0 {
		return nil, ErrLBNotReady
	}
	return externalIPs, nil
}

func getJoinServer(ctx context.Context, cli client.Client, mesh *meshv1.Mesh, thisGroup *meshv1.NodeGroup) (string, error) {
	// TODO: We should technically list all node groups
	var bootstrapGroup meshv1.NodeGroupList
	err := cli.List(ctx, &bootstrapGroup,
		client.InNamespace(mesh.GetNamespace()),
		client.MatchingLabels(meshv1.MeshBootstrapGroupSelector(mesh)))
	if err != nil {
		return "", fmt.Errorf("list bootstrap node group: %w", err)
	}
	if len(bootstrapGroup.Items) == 0 {
		return "", fmt.Errorf("no bootstrap node group found")
	}
	for _, group := range bootstrapGroup.Items {
		if group.Name == thisGroup.Name {
			continue
		}
		if group.Spec.Cluster.Service != nil {
			externalURLs, err := getLBExternalIPs(ctx, cli, mesh, &group)
			if err != nil {
				return "", fmt.Errorf("get load balancer external IP: %w", err)
			}
			return fmt.Sprintf(`%s:%d`, externalURLs[0], group.Spec.Cluster.Service.GRPCPort), nil
		}
	}
	// Fall back to headless service only if this is one of the bootstrap groups
	var joinServer string
	if labels := thisGroup.GetLabels(); labels != nil && labels[meshv1.BootstrapNodeGroupLabel] == "true" {
		for _, group := range bootstrapGroup.Items {
			if group.Name == thisGroup.Name {
				continue
			}
			joinServer = fmt.Sprintf(`%s:%d`, meshv1.MeshNodeGroupHeadlessServiceFQDN(mesh, &group), meshv1.DefaultGRPCPort)
		}
	}
	if joinServer == "" {
		return "", fmt.Errorf("no join server found")
	}
	return joinServer, nil
}

func pointer[T any](v T) *T {
	return &v
}
