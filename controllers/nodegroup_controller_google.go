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
	"net/http"
	"strings"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	meshv1 "github.com/webmeshproj/operator/api/v1"
	"github.com/webmeshproj/operator/controllers/cloudconfig"
	"github.com/webmeshproj/operator/controllers/nodeconfig"
)

func (r *NodeGroupReconciler) reconcileGoogleCloudNodeGroup(ctx context.Context, mesh *meshv1.Mesh, group *meshv1.NodeGroup) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	opts, err := r.getGoogleClientOptions(ctx, group)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Create clients
	images, err := compute.NewImageFamilyViewsRESTClient(ctx, opts...)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create compute images client: %w", err)
	}
	defer images.Close()
	subnets, err := compute.NewSubnetworksRESTClient(ctx, opts...)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create compute subnetworks client: %w", err)
	}
	defer subnets.Close()
	instances, err := compute.NewInstancesRESTClient(ctx, opts...)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create compute instances client: %w", err)
	}
	defer instances.Close()

	spec := group.Spec.GoogleCloud

	// Fetch the latest ubuntu boot image
	bootImage, err := images.Get(ctx, &computepb.GetImageFamilyViewRequest{
		Family:  "ubuntu-2204-lts",
		Project: "ubuntu-os-cloud",
		Zone:    spec.Zone,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get latest ubuntu image: %w", err)
	}

	// Fetch the subnet
	subnet, err := subnets.Get(ctx, &computepb.GetSubnetworkRequest{
		Project: spec.ProjectID,
		Region: func() string {
			if spec.Region != "" {
				return spec.Region
			}
			zone := strings.Split(spec.Zone, "-")
			return strings.Join(zone[:len(zone)-1], "-")
		}(),
		Subnetwork: spec.Subnetwork,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get subnet: %w", err)
	}

	// Build the nodeconfig
	joinServer, err := getJoinServer(ctx, r.Client, mesh, group)
	if err != nil {
		if errors.Is(err, ErrLBNotReady) {
			log.Info("load balancer not ready, requeueing")
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Second * 3,
			}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get join server: %w", err)
	}
	nodeconf, err := nodeconfig.New(nodeconfig.Options{
		Mesh:                 mesh,
		Group:                group,
		JoinServer:           joinServer,
		IsPersistent:         true,
		CertDir:              meshv1.DefaultTLSDirectory,
		DetectEndpoints:      true,
		AllowRemoteDetection: true,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("build node config: %w", err)
	}

	// Loop over replicas and ensure each instance
	for i := 0; i < int(*group.Spec.Replicas); i++ {
		name := fmt.Sprintf("%s-%d", group.GetName(), i)

		// Get the certificate secret for this node
		var secret corev1.Secret
		err = r.Get(ctx, client.ObjectKey{
			Name:      meshv1.MeshNodeCertName(mesh, group, i),
			Namespace: group.GetNamespace(),
		}, &secret)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("get node certificate secret: %w", err)
		}
		for _, key := range []string{corev1.TLSCertKey, corev1.TLSPrivateKeyKey, cmmeta.TLSCAKey} {
			if _, ok := secret.Data[key]; !ok {
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: time.Second * 3,
				}, fmt.Errorf("node certificate secret missing key %q", key)
			}
		}
		// Build the cloud config
		cloudconf, err := cloudconfig.New(cloudconfig.Options{
			Image:   group.Spec.Image,
			Config:  nodeconf,
			TLSCert: secret.Data[corev1.TLSCertKey],
			TLSKey:  secret.Data[corev1.TLSPrivateKeyKey],
			CA:      secret.Data[cmmeta.TLSCAKey],
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("build cloud config: %w", err)
		}
		description := fmt.Sprintf("%s %s", name, cloudconf.Checksum())

		// Ensure the instance
		instance, err := instances.Get(ctx, &computepb.GetInstanceRequest{
			Project:  spec.ProjectID,
			Zone:     spec.Zone,
			Instance: name,
		})
		if err == nil {
			log.Info("Node instance already exists", "name", instance.GetName())
			if instance.GetDescription() != description {
				// Delete the instance and recreate it
				log.Info("Config checksum has changed, deleting instance", "name", instance.GetName())
				op, err := instances.Delete(ctx, &computepb.DeleteInstanceRequest{
					Project:  spec.ProjectID,
					Zone:     spec.Zone,
					Instance: name,
				})
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("delete instance: %w", err)
				}
				if err := op.Wait(ctx); err != nil {
					return ctrl.Result{}, fmt.Errorf("wait for instance delete: %w", err)
				}
			} else {
				log.Info("Config checksum has not changed, skipping instance", "name", instance.GetName())
				continue
			}
		} else {
			gerr := &googleapi.Error{}
			ok := errors.As(err, &gerr)
			if (ok && gerr.Code != http.StatusNotFound) || !ok {
				return ctrl.Result{}, fmt.Errorf("lookup existing instance: %w", err)
			}
		}
		log.Info("Creating instance", "name", name)
		instanceReq := &computepb.InsertInstanceRequest{
			Project: spec.ProjectID,
			Zone:    spec.Zone,
			InstanceResource: &computepb.Instance{
				Name:         &name,
				Description:  &description,
				MachineType:  pointer(fmt.Sprintf("zones/%s/machineTypes/%s", spec.Zone, spec.MachineType)),
				Labels:       group.GetLabels(),
				CanIpForward: pointer(true),
				AdvancedMachineFeatures: &computepb.AdvancedMachineFeatures{
					EnableUefiNetworking: pointer(true),
				},
				Disks: []*computepb.AttachedDisk{
					{
						Boot:       pointer(true),
						AutoDelete: pointer(true),
						InitializeParams: &computepb.AttachedDiskInitializeParams{
							SourceImage: bootImage.Image.SelfLink,
						},
					},
				},
				Metadata: &computepb.Metadata{
					Items: []*computepb.Items{
						{
							Key:   pointer("user-data"),
							Value: pointer(string(cloudconf.Raw())),
						},
					},
				},
				NetworkInterfaces: []*computepb.NetworkInterface{
					{
						Subnetwork: subnet.SelfLink,
						StackType:  pointer("IPV4_IPV6"),
						AccessConfigs: []*computepb.AccessConfig{{
							Name: pointer("wanv4"),
						}},
						Ipv6AccessConfigs: []*computepb.AccessConfig{
							{
								Name:        pointer("wanv6"),
								Type:        pointer("DIRECT_IPV6"),
								NetworkTier: pointer("PREMIUM"),
							},
						},
					},
				},
				Tags: &computepb.Tags{
					Items: spec.Tags,
				},
			},
		}
		op, err := instances.Insert(ctx, instanceReq)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create instance: %w", err)
		}
		err = op.Wait(ctx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("wait for instance creation: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *NodeGroupReconciler) deleteGoogleCloudNodeGroup(ctx context.Context, group *meshv1.NodeGroup) error {
	spec := group.Spec.GoogleCloud
	opts, err := r.getGoogleClientOptions(ctx, group)
	if err != nil {
		return fmt.Errorf("get google client options: %w", err)
	}
	instances, err := compute.NewInstancesRESTClient(ctx, opts...)
	if err != nil {
		return fmt.Errorf("create compute instances client: %w", err)
	}
	defer instances.Close()
	for i := 0; i < int(*group.Spec.Replicas); i++ {
		name := fmt.Sprintf("%s-%d", group.GetName(), i)
		// Check if the instance already exists
		instance, err := instances.Get(ctx, &computepb.GetInstanceRequest{
			Project:  spec.ProjectID,
			Zone:     spec.Zone,
			Instance: name,
		})
		if err == nil {
			// Delete the instance
			log.FromContext(ctx).Info("Deleting node group instance", "name", name)
			op, err := instances.Delete(ctx, &computepb.DeleteInstanceRequest{
				Project:  spec.ProjectID,
				Zone:     spec.Zone,
				Instance: instance.GetName(),
			})
			if err != nil {
				return fmt.Errorf("delete instance: %w", err)
			}
			if err := op.Wait(ctx); err != nil {
				return fmt.Errorf("wait for instance deletion: %w", err)
			}
		} else {
			gerr := &googleapi.Error{}
			ok := errors.As(err, &gerr)
			if (ok && gerr.Code != http.StatusNotFound) || !ok {
				return fmt.Errorf("failed to lookup existing instance: %w", err)
			}
		}
	}
	return nil
}

func (r *NodeGroupReconciler) getGoogleClientOptions(ctx context.Context, group *meshv1.NodeGroup) ([]option.ClientOption, error) {
	if group.Spec.GoogleCloud.Credentials == nil {
		// We assume workload identity is enabled
		return nil, nil
	}
	var secret corev1.Secret
	err := r.Get(ctx, client.ObjectKey{
		Name:      group.Spec.GoogleCloud.Credentials.Name,
		Namespace: group.GetNamespace(),
	}, &secret)
	if err != nil {
		return nil, err
	}
	key, ok := secret.Data[group.Spec.GoogleCloud.Credentials.Key]
	if !ok {
		return nil, fmt.Errorf("no key %s in secret %s/%s",
			group.Spec.GoogleCloud.Credentials.Key, group.GetNamespace(), group.Spec.GoogleCloud.Credentials.Name)
	}
	return []option.ClientOption{option.WithCredentialsJSON(key)}, nil
}
