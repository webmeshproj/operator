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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	meshv1 "github.com/webmeshproj/operator/api/v1"
	"github.com/webmeshproj/operator/controllers/nodeconfig"
	"github.com/webmeshproj/operator/controllers/resources"
)

func (r *NodeGroupReconciler) reconcileClusterNodeGroup(ctx context.Context, mesh *meshv1.Mesh, group *meshv1.NodeGroup) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling cluster node group")

	toApply := make([]client.Object, 0)
	cli := r.Client
	if group.Spec.Cluster.Kubeconfig != nil {
		// TODO: Doesn't account for certificates needing to be copied
		// to the remote cluster
		var secret corev1.Secret
		err := r.Get(ctx, client.ObjectKey{
			Name:      group.Spec.Cluster.Kubeconfig.Name,
			Namespace: group.GetNamespace(),
		}, &secret)
		if err != nil {
			log.Error(err, "unable to fetch kubeconfig secret")
			return ctrl.Result{}, err
		}
		kubeconfig, ok := secret.Data[group.Spec.Cluster.Kubeconfig.Key]
		if !ok {
			err := errors.New("kubeconfig secret does not contain key")
			log.Error(err, "unable to fetch kubeconfig secret")
			return ctrl.Result{}, err
		}
		cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
		if err != nil {
			log.Error(err, "unable to create client config")
			return ctrl.Result{}, err
		}
		cli, err = client.New(cfg, client.Options{})
		if err != nil {
			log.Error(err, "unable to create client")
			return ctrl.Result{}, err
		}
	}

	// Create the service if we are exposing the node group
	var externalURLs []string
	if group.Spec.Cluster.Service != nil {
		lbConfig, csum, err := resources.NewNodeGroupLBConfigMap(mesh, group)
		if err != nil {
			return ctrl.Result{}, err
		}
		lbDeploy := resources.NewNodeGroupLBDeployment(mesh, group, csum)
		toApply = append(toApply, lbConfig, lbDeploy, resources.NewNodeGroupLBService(mesh, group))
		if group.Spec.Cluster.Service.ExternalURL != "" {
			externalURLs = []string{group.Spec.Cluster.Service.ExternalURL}
		} else {
			// We need to pre-create the service so we can use it as the external URL
			err := resources.Apply(ctx, cli, toApply)
			if err != nil {
				log.Error(err, "unable to apply resources")
				return ctrl.Result{}, err
			}
			lbIPs, err := getLBExternalIPs(ctx, cli, mesh, group)
			if err != nil {
				if errors.Is(err, ErrLBNotReady) {
					log.Info("waiting for load balancer to be ready")
					return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
				}
				log.Error(err, "unable to get load balancer external IP")
				return ctrl.Result{}, err
			}
			externalURLs = append(externalURLs, lbIPs...)
			// Reset toApply
			toApply = make([]client.Object, 0)
		}
	}

	// Create Node group volumes and configmaps
	toApply = append(toApply, resources.NewNodeGroupHeadlessService(mesh, group))
	confs := make([]*nodeconfig.Config, *group.Spec.Replicas)
	for i := 0; i < int(*group.Spec.Replicas); i++ {
		if group.Spec.Cluster.PVCSpec != nil {
			toApply = append(toApply, resources.NewNodeGroupPersistentVolumeClaim(mesh, group, i))
		}
		conf, err := r.buildClusterNodeConfig(ctx, mesh, group, externalURLs, i)
		if err != nil {
			return ctrl.Result{}, err
		}
		confs[i] = conf
		toApply = append(toApply, resources.NewNodeGroupConfigMap(mesh, group, conf, i))
	}

	if err := resources.Apply(ctx, cli, toApply); err != nil {
		log.Error(err, "unable to apply resources")
		return ctrl.Result{}, err
	}

	// Handle pods individually
	// var needsRequeue bool
	for i := 0; i < int(*group.Spec.Replicas); i++ {
		pod, err := resources.NewNodeGroupPod(mesh, group, confs[i].Checksum(), i)
		if err != nil {
			return ctrl.Result{}, err
		}
		// Check if the pod exists
		var existing corev1.Pod
		err = cli.Get(ctx, client.ObjectKey{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		}, &existing)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				log.Error(err, "unable to get pod")
				return ctrl.Result{}, err
			}
			// Apply the pod
			err := resources.Apply(ctx, cli, []client.Object{pod})
			if err != nil {
				log.Error(err, "unable to apply pod")
				return ctrl.Result{}, err
			}
			// needsRequeue = true
		} else {
			// Check if the spec has changed
			existingConfSum, confSumOk := existing.Annotations[meshv1.ConfigChecksumAnnotation]
			existingSpecSum, specSumOk := existing.Annotations[meshv1.SpecChecksumAnnotation]
			if !confSumOk || !specSumOk || existingConfSum != confs[i].Checksum() || existingSpecSum != pod.Annotations[meshv1.SpecChecksumAnnotation] {
				// TODO: Depending on the role of the group - this may need to be done gracefully
				// Delete the pod
				err := cli.Delete(ctx, &existing)
				if err != nil {
					log.Error(err, "unable to delete pod")
					return ctrl.Result{}, err
				}
				// Apply the pod
				err = resources.Apply(ctx, cli, []client.Object{pod})
				if err != nil {
					log.Error(err, "unable to apply pod")
					return ctrl.Result{}, err
				}
				// needsRequeue = true
			}
			// // Create endpoint slices for the pod
			// endpoints, err := resources.NewNodeGroupPodEndpointSlices(mesh, group, &existing, i)
			// if err != nil {
			// 	return ctrl.Result{}, err
			// }
			// eps := make([]client.Object, len(endpoints))
			// for i, ep := range endpoints {
			// 	eps[i] = ep
			// }
			// err = resources.Apply(ctx, cli, eps)
			// if err != nil {
			// 	log.Error(err, "unable to apply endpoint slices")
			// 	return ctrl.Result{}, err
			// }
		}
	}

	// if needsRequeue {
	// 	return ctrl.Result{Requeue: true, RequeueAfter: time.Second}, nil
	// }

	return ctrl.Result{}, nil
}

func (r *NodeGroupReconciler) buildClusterNodeConfig(ctx context.Context, mesh *meshv1.Mesh, group *meshv1.NodeGroup, externalURLs []string, index int) (*nodeconfig.Config, error) {
	var isBootstrap bool
	if val, ok := group.GetAnnotations()[meshv1.BootstrapNodeGroupAnnotation]; ok && val == "true" {
		isBootstrap = true
	}
	var primaryEndpoint string
	internalEndpoint := fmt.Sprintf(`{{ env "POD_NAME" }}.%s:%d`, meshv1.MeshNodeGroupHeadlessServiceFQDN(mesh, group), meshv1.DefaultWireGuardPort+index)
	wireguardEndpoints := []string{internalEndpoint}
	if len(externalURLs) > 0 {
		primaryEndpoint = externalURLs[0]
		startPort := func() int {
			if group.Spec.Cluster.Service != nil {
				return int(group.Spec.Cluster.Service.WireGuardPort)
			}
			return meshv1.DefaultWireGuardPort
		}()
		for _, url := range externalURLs {
			addr, err := netip.ParseAddr(url)
			if err != nil {
				return nil, err
			}
			var externalEndpoint string
			if addr.Is4() {
				externalEndpoint = fmt.Sprintf(`%s:%d`, url, startPort+index)
			} else {
				externalEndpoint = fmt.Sprintf(`[%s]:%d`, url, startPort+index)
			}
			wireguardEndpoints = append(wireguardEndpoints, externalEndpoint)
		}
	} else if isBootstrap {
		primaryEndpoint = fmt.Sprintf(`{{ env "POD_NAME" }}.%s`, meshv1.MeshNodeGroupHeadlessServiceFQDN(mesh, group))
	}
	var advertiseAddress string
	var joinServer string
	bootstrapServers := make(map[string]string)
	if isBootstrap {
		if *group.Spec.Replicas > 1 {
			advertiseAddress = fmt.Sprintf(`{{ env "POD_NAME" }}.%s:%d`, meshv1.MeshNodeGroupHeadlessServiceFQDN(mesh, group), meshv1.DefaultRaftPort)
			for i := 0; i < int(*group.Spec.Replicas); i++ {
				bootstrapServers[meshv1.MeshNodeHostname(mesh, group, i)] = fmt.Sprintf("%s:%d", meshv1.MeshNodeClusterFQDN(mesh, group, i), meshv1.DefaultRaftPort)
			}
		}
	} else {
		var err error
		joinServer, err = getJoinServer(ctx, r.Client, mesh)
		if err != nil {
			return nil, fmt.Errorf("get join server: %w", err)
		}
	}
	conf, err := nodeconfig.New(nodeconfig.Options{
		Mesh:                mesh,
		Group:               group,
		AdvertiseAddress:    advertiseAddress,
		PrimaryEndpoint:     primaryEndpoint,
		WireGuardEndpoints:  wireguardEndpoints,
		IsBootstrap:         isBootstrap,
		BootstrapServers:    bootstrapServers,
		JoinServer:          joinServer,
		IsPersistent:        group.Spec.Cluster.PVCSpec != nil,
		CertDir:             meshv1.DefaultTLSDirectory,
		WireGuardListenPort: meshv1.DefaultWireGuardPort + index,
	})
	if err != nil {
		return nil, fmt.Errorf("build node config: %w", err)
	}
	return conf, nil
}