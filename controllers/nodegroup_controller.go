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
	"time"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	meshv1 "github.com/webmeshproj/operator/api/v1"
	"github.com/webmeshproj/operator/controllers/resources"
)

// NodeGroupReconciler reconciles a NodeGroup object
type NodeGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims;services;configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mesh.webmesh.io,resources=nodegroups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mesh.webmesh.io,resources=nodegroups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mesh.webmesh.io,resources=nodegroups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *NodeGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var group meshv1.NodeGroup
	if err := r.Get(ctx, req.NamespacedName, &group); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch NodeGroup")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("reconciling NodeGroup")
	toApply := make([]client.Object, 0)

	// Get the mesh object
	var mesh meshv1.Mesh
	if err := r.Get(ctx, client.ObjectKey{
		Name: group.Spec.Mesh.Name,
		Namespace: func() string {
			if group.Spec.Mesh.Namespace != "" {
				return group.Spec.Mesh.Namespace
			}
			return group.GetNamespace()
		}(),
	}, &mesh); err != nil {
		log.Error(err, "unable to fetch Mesh")
		return ctrl.Result{}, err
	}

	// Create the service if we are exposing the node group
	var externalURL string
	if group.Spec.Cluster.Service != nil {
		lbconfig, checksum, err := resources.NewNodeGroupLBConfigMap(&mesh, &group)
		if err != nil {
			log.Error(err, "unable to create config map")
			return ctrl.Result{}, err
		}
		toApply = append(toApply, lbconfig,
			resources.NewNodeGroupLBDeployment(&mesh, &group, checksum),
			resources.NewNodeGroupLBService(&mesh, &group))
		externalURL = group.Spec.Cluster.Service.ExternalURL
		if externalURL == "" && group.Spec.Cluster.Service.Type != corev1.ServiceTypeClusterIP {
			// We need to pre-create the service so we can use it as the external URL
			err = resources.Apply(ctx, r.Client, toApply)
			if err != nil {
				log.Error(err, "unable to apply resources")
				return ctrl.Result{}, err
			}
			externalURL, err = getLBExternalIP(ctx, r.Client, &mesh, &group)
			if err != nil {
				if errors.Is(err, ErrLBNotReady) {
					log.Info("waiting for load balancer to be ready")
					return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
				}
				log.Error(err, "unable to get load balancer external IP")
				return ctrl.Result{}, err
			}
			// Reset toApply
			toApply = make([]client.Object, 0)
		}
	}

	// Create Node group resources
	toApply = append(toApply, resources.NewNodeGroupHeadlessService(&mesh, &group))
	for i := 0; i < int(group.Spec.Cluster.Replicas); i++ {
		toApply = append(toApply, resources.NewNodeCertificate(&mesh, &group, i))
	}
	var isBootstrap bool
	if val, ok := group.GetAnnotations()[meshv1.BootstrapNodeGroupAnnotation]; ok && val == "true" {
		isBootstrap = true
	}
	configMap, checksum, err := resources.NewNodeGroupConfigMap(resources.NodeGroupConfigOptions{
		Mesh:             &mesh,
		Group:            &group,
		IsBootstrap:      isBootstrap,
		ExternalEndpoint: externalURL,
	})
	if err != nil {
		log.Error(err, "unable to create config map")
		return ctrl.Result{}, err
	}
	toApply = append(toApply, configMap,
		resources.NewNodeGroupStatefulSet(&mesh, &group, checksum))

	// Apply any remaining resources
	if err := resources.Apply(ctx, r.Client, toApply); err != nil {
		log.Error(err, "unable to apply resources")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&meshv1.NodeGroup{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&appsv1.Deployment{}).
		Owns(&certv1.Certificate{}).
		Complete(r)
}
