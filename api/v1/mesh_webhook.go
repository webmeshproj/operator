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

package v1

import (
	"context"

	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var meshlog = logf.Log.WithName("mesh-resource")

func (r *Mesh) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithValidator(&meshValidator{Client: mgr.GetClient()}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-mesh-webmesh-io-v1-mesh,mutating=true,failurePolicy=fail,sideEffects=None,groups=mesh.webmesh.io,resources=meshes,verbs=create;update,versions=v1,name=mmesh.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Mesh{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Mesh) Default() {
	meshlog.Info("defaulting", "name", r.Name)

	// Ensure a default config for the bootstrap node group
	if r.Spec.Bootstrap.Config == nil && r.Spec.Bootstrap.ConfigGroup == "" {
		var nodegroupConfig NodeGroupConfig
		nodegroupConfig.Default()
		r.Spec.Bootstrap.Config = &nodegroupConfig
	} else if r.Spec.Bootstrap.Config != nil {
		r.Spec.Bootstrap.Config.Default()
	}
	// TODO: Handle non-cluster bootstrap groups
	if r.Spec.Bootstrap.Cluster == nil {
		r.Spec.Bootstrap.Cluster = &NodeGroupClusterConfig{}
	}
	r.Spec.Bootstrap.Cluster.Default()
	if r.Spec.Bootstrap.Cluster.PVCSpec == nil {
		// Require persistence for the bootstrap node group
		r.Spec.Bootstrap.Cluster.PVCSpec = &corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(DefaultStorageSize),
				},
			},
		}
	}

	// Set the issuer name if we are creating it
	if r.Spec.Issuer.Create {
		r.Spec.Issuer.IssuerRef = cmmeta.ObjectReference{
			Name: MeshCAName(r),
			Kind: r.Spec.Issuer.Kind,
		}
	}
}

//+kubebuilder:webhook:path=/validate-mesh-webmesh-io-v1-mesh,mutating=false,failurePolicy=fail,sideEffects=None,groups=mesh.webmesh.io,resources=meshes,verbs=create;update,versions=v1,name=vmesh.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &meshValidator{}

type meshValidator struct {
	client.Client
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *meshValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	o := obj.(*Mesh)
	warnings := make(admission.Warnings, 0)
	meshlog.Info("validating create", "name", o.Name)

	// Validate bootstrap node group
	if o.Spec.Bootstrap.ConfigGroup != "" {
		if _, ok := o.Spec.ConfigGroups[o.Spec.Bootstrap.ConfigGroup]; !ok {
			return nil, field.Invalid(
				field.NewPath("spec", "bootstrap", "configGroup"),
				o.Spec.Bootstrap.ConfigGroup,
				"configGroup must be a valid config group name")
		}
	}

	// Validate Issuer configurations
	if o.Spec.Issuer.IssuerRef.Name == "" {
		if !o.Spec.Issuer.Create {
			return nil, field.Invalid(
				field.NewPath("spec", "issuer", "create"),
				o.Spec.Issuer.Create,
				"create must be true if issuerRef.name is empty")
		}
	} else {
		if o.Spec.Issuer.IssuerRef.Kind == "" {
			return nil, field.Invalid(
				field.NewPath("spec", "issuer", "issuerRef", "kind"),
				o.Spec.Issuer.IssuerRef.Kind,
				"kind must not be empty if issuerRef.name is not empty")
		}
	}

	return warnings, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *meshValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	old := oldObj.(*Mesh)
	new := newObj.(*Mesh)
	meshlog.Info("validating update", "name", old.Name)
	if old.Spec.IPv4 != new.Spec.IPv4 {
		return nil, field.Invalid(
			field.NewPath("spec", "ipv4"),
			new.Spec.IPv4,
			"ipv4 is immutable")
	}
	if old.Spec.Bootstrap.Cluster != nil {
		if old.Spec.Bootstrap.Cluster.Replicas != new.Spec.Bootstrap.Cluster.Replicas {
			return nil, field.Invalid(
				field.NewPath("spec", "bootstrap", "replicas"),
				new.Spec.Bootstrap.Cluster.Replicas,
				"bootstrap.replicas is immutable")
		}
		if old.Spec.Bootstrap.Cluster.PVCSpec != nil && new.Spec.Bootstrap.Cluster.PVCSpec == nil {
			return nil, field.Invalid(
				field.NewPath("spec", "bootstrap", "pvcSpec"),
				new.Spec.Bootstrap.Cluster.PVCSpec,
				"changing to a non-persistent bootstrap node group is not supported")
		} else if old.Spec.Bootstrap.Cluster.PVCSpec == nil && new.Spec.Bootstrap.Cluster.PVCSpec != nil {
			return nil, field.Invalid(
				field.NewPath("spec", "bootstrap", "pvcSpec"),
				new.Spec.Bootstrap.Cluster.PVCSpec,
				"changing to a persistent bootstrap node group is not supported")
		}
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *meshValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	o := obj.(*Mesh)
	meshlog.Info("validating delete", "name", o.Name)
	return nil, nil
}
