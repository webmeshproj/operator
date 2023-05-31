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
	"encoding/base64"
	"fmt"
	"time"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	ctlconfig "github.com/webmeshproj/node/pkg/ctlcmd/config"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims;services;configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
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

	// Create the admin certificate
	toApply = append(toApply, resources.NewMeshAdminCertificate(&mesh))

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

	// Apply the resources
	if err := resources.Apply(ctx, r.Client, toApply); err != nil {
		log.Error(err, "unable to apply resources")
		return ctrl.Result{}, err
	}

	if bootstrapGroup.Spec.Service == nil || bootstrapGroup.Spec.Service.Type == corev1.ServiceTypeClusterIP {
		// We are done here, we can't generate an admin config
		// without an exposed service
		return ctrl.Result{}, nil
	}

	return r.buildAdminConfig(ctx, &mesh)
}

func (r *MeshReconciler) buildAdminConfig(ctx context.Context, mesh *meshv1.Mesh) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	// Get the LB service
	var lbService corev1.Service
	err := r.Get(ctx, client.ObjectKey{
		Name:      meshv1.MeshNodeGroupLBName(mesh, mesh.BootstrapGroup()),
		Namespace: mesh.GetNamespace(),
	}, &lbService)
	if err != nil {
		log.Error(err, "unable to fetch load balancer service")
		return ctrl.Result{}, err
	}
	var externalIP string
	switch lbService.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		if len(lbService.Status.LoadBalancer.Ingress) == 0 {
			log.Info("load balancer service has no ingress, requeueing")
			return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 3}, nil
		}
		externalIP = lbService.Status.LoadBalancer.Ingress[0].IP
	case corev1.ServiceTypeNodePort:
		// TODO: This is not correct, we need to get the external IP of the node
		externalIP = lbService.Spec.ClusterIP
	default:
		log.Info("load balancer service has unknown type, requeueing")
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 3}, nil
	}
	// Get the admin certificate
	var secret corev1.Secret
	err = r.Get(ctx, client.ObjectKey{
		Name:      meshv1.MeshAdminCertName(mesh),
		Namespace: mesh.GetNamespace(),
	}, &secret)
	if err != nil {
		log.Error(err, "unable to fetch admin certificate secret")
		return ctrl.Result{}, err
	}
	for _, key := range []string{corev1.TLSCertKey, corev1.TLSPrivateKeyKey, cmmeta.TLSCAKey} {
		if data, ok := secret.Data[key]; !ok || len(data) == 0 {
			log.Info("admin certificate secret missing data, requeueing")
			return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 3}, nil
		}
	}

	// Create a config for the admin
	adminConfig := ctlconfig.New()
	adminConfig.Clusters = []ctlconfig.Cluster{
		{
			Name: mesh.GetName(),
			Cluster: ctlconfig.ClusterConfig{
				Server:                   fmt.Sprintf("%s:%d", externalIP, mesh.Spec.Bootstrap.Service.GRPCPort),
				TLSVerifyChainOnly:       true,
				CertificateAuthorityData: base64.StdEncoding.EncodeToString(secret.Data[cmmeta.TLSCAKey]),
			},
		},
	}
	adminConfig.Users = []ctlconfig.User{
		{
			Name: mesh.GetName() + "-admin",
			User: ctlconfig.UserConfig{
				ClientCertificateData: base64.StdEncoding.EncodeToString(secret.Data[corev1.TLSCertKey]),
				ClientKeyData:         base64.StdEncoding.EncodeToString(secret.Data[corev1.TLSPrivateKeyKey]),
			},
		},
	}
	adminConfig.Contexts = []ctlconfig.Context{
		{
			Name: mesh.GetName(),
			Context: ctlconfig.ContextConfig{
				Cluster: mesh.GetName(),
				User:    mesh.GetName() + "-admin",
			},
		},
	}
	adminConfig.CurrentContext = mesh.GetName()
	data, err := yaml.Marshal(adminConfig)
	if err != nil {
		log.Error(err, "unable to marshal admin config")
		return ctrl.Result{}, err
	}

	// Create a secret for the admin config
	adminConfigSecret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            meshv1.MeshAdminConfigName(mesh),
			Namespace:       mesh.GetNamespace(),
			Labels:          meshv1.MeshLabels(mesh),
			Annotations:     mesh.GetAnnotations(),
			OwnerReferences: meshv1.OwnerReferences(mesh),
		},
		Data: map[string][]byte{
			"config.yaml": data,
		},
	}
	if err := resources.Apply(ctx, r.Client, []client.Object{&adminConfigSecret}); err != nil {
		log.Error(err, "unable to apply admin config secret")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MeshReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&meshv1.Mesh{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&certv1.ClusterIssuer{}).
		Owns(&certv1.Issuer{}).
		Owns(&certv1.Certificate{}).
		Complete(r)
}
