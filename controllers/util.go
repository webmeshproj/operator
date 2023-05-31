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

	meshv1 "github.com/webmeshproj/operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var ErrLBNotReady = errors.New("load balancer not ready")

func getLBExternalIP(ctx context.Context, cli client.Client, mesh *meshv1.Mesh, group *meshv1.NodeGroup) (string, error) {
	var lbService corev1.Service
	err := cli.Get(ctx, client.ObjectKey{
		Name:      meshv1.MeshNodeGroupLBName(mesh, group),
		Namespace: mesh.GetNamespace(),
	}, &lbService)
	if err != nil {
		return "", fmt.Errorf("fetch load balancer service: %w", err)
	}
	switch lbService.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		if len(lbService.Status.LoadBalancer.Ingress) == 0 {
			return "", ErrLBNotReady
		}
		return lbService.Status.LoadBalancer.Ingress[0].IP, nil
	case corev1.ServiceTypeNodePort:
		// TODO: This is not correct, we need to get the external IP of the node
		return lbService.Spec.ClusterIP, nil
	case corev1.ServiceTypeClusterIP:
		return lbService.Spec.ClusterIP, nil
	default:
		return "", fmt.Errorf("service has unknown type: %s", lbService.Spec.Type)
	}
}
