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

// MeshReconciler reconciles a Mesh object
type MeshReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims;services;configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cert-manager.io,resources=clusterissuers;issuers;certificates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mesh.webmesh.io,resources=meshes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mesh.webmesh.io,resources=meshes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mesh.webmesh.io,resources=meshes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MeshReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var mesh meshv1.Mesh
	if err := r.Get(ctx, req.NamespacedName, &mesh); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch Mesh")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling Mesh")
	toApply := make([]client.Object, 0)

	// Create an issuer if requested
	if mesh.Spec.Issuer.Create {
		toApply = append(toApply,
			resources.NewMeshSelfSigner(&mesh),
			resources.NewMeshCACertificate(&mesh),
			resources.NewMeshIssuer(&mesh),
		)
	}

	// Configure the bootstrap group
	bootstrapGroup := mesh.BootstrapGroup()
	toApply = append(toApply, resources.NewNodeGroupHeadlessService(&mesh, bootstrapGroup, &mesh))
	for i := 0; i < int(bootstrapGroup.Spec.Replicas); i++ {
		toApply = append(toApply, resources.NewNodeCertificate(&mesh, bootstrapGroup, &mesh, i))
	}
	configMap, err := resources.NewNodeGroupConfigMap(&mesh, bootstrapGroup, &mesh, true)
	if err != nil {
		log.Error(err, "unable to create config map")
		return ctrl.Result{}, err
	}
	checksum := configMap.GetAnnotations()[meshv1.ConfigChecksumAnnotation]
	toApply = append(toApply, configMap,
		resources.NewNodeGroupStatefulSet(&mesh, bootstrapGroup, &mesh, checksum))

	if bootstrapGroup.Spec.Service != nil {
		lbconfig, err := resources.NewNodeGroupLBConfigMap(&mesh, bootstrapGroup, &mesh)
		if err != nil {
			log.Error(err, "unable to create config map")
			return ctrl.Result{}, err
		}
		toApply = append(toApply, lbconfig,
			resources.NewNodeGroupLBDeployment(&mesh, bootstrapGroup, &mesh),
			resources.NewNodeGroupLBService(&mesh, bootstrapGroup, &mesh))
	}

	if err := resources.Apply(ctx, r.Client, toApply); err != nil {
		log.Error(err, "unable to apply resources")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MeshReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&meshv1.Mesh{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&certv1.ClusterIssuer{}).
		Owns(&certv1.Issuer{}).
		Owns(&certv1.Certificate{}).
		Complete(r)
}
