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
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	ctlconfig "github.com/webmeshproj/node/pkg/ctlcmd/config"
	"gopkg.in/yaml.v3"
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

// TODO: Lookup referenced groups and delete them too
const meshesForegroundDeletion = "meshes.mesh.webmesh.io"

//+kubebuilder:rbac:groups="",resources=services;secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cert-manager.io,resources=clusterissuers;issuers;certificates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mesh.webmesh.io,resources=nodegroups,verbs=get;list;watch;create;update;patch;delete
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

	// Create the bootstrap group
	bootstraps := mesh.BootstrapGroups()
	for _, group := range bootstraps {
		toApply = append(toApply, group)
	}

	// Apply the resources
	if err := resources.Apply(ctx, r.Client, toApply); err != nil {
		log.Error(err, "unable to apply resources")
		return ctrl.Result{}, err
	}

	// Find the public bootstrap group, if any
	var publicBootstrap *meshv1.NodeGroup
	for _, group := range bootstraps {
		if group.Spec.Cluster != nil && group.Spec.Cluster.Service != nil {
			publicBootstrap = group
			break
		}
	}

	if publicBootstrap == nil {
		// We are done here, we can't generate an admin config
		// without an exposed service
		return ctrl.Result{}, nil
	}

	return r.buildAdminConfig(ctx, &mesh, publicBootstrap)
}

func (r *MeshReconciler) buildAdminConfig(ctx context.Context, mesh *meshv1.Mesh, group *meshv1.NodeGroup) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	// Get the LB service
	externalIPs, err := getLBExternalIPs(ctx, r.Client, mesh, group)
	if err != nil {
		if errors.Is(err, ErrLBNotReady) {
			log.Info("LB not ready, requeueing")
			return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 3}, nil
		}
		log.Error(err, "unable to get LB external IP")
		return ctrl.Result{}, err
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
				Server:                   fmt.Sprintf("%s:%d", externalIPs[0], mesh.Spec.Bootstrap.Cluster.Service.GRPCPort),
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

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	err = enc.Encode(adminConfig)
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
			"config.yaml": buf.Bytes(),
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
		Owns(&meshv1.NodeGroup{}).
		Owns(&corev1.Secret{}).
		Owns(&certv1.ClusterIssuer{}).
		Owns(&certv1.Issuer{}).
		Owns(&certv1.Certificate{}).
		Complete(r)
}
