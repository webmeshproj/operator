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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var nodegrouplog = logf.Log.WithName("nodegroup-resource")

func (r *NodeGroup) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithValidator(&nodeGroupValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-mesh-webmesh-io-v1-nodegroup,mutating=true,failurePolicy=fail,sideEffects=None,groups=mesh.webmesh.io,resources=nodegroups,verbs=create;update,versions=v1,name=mnodegroup.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &NodeGroup{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *NodeGroup) Default() {
	nodegrouplog.Info("defaulting", "name", r.Name)
	r.Spec.Default()
}

//+kubebuilder:webhook:path=/validate-mesh-webmesh-io-v1-nodegroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=mesh.webmesh.io,resources=nodegroups,verbs=create;update,versions=v1,name=vnodegroup.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &nodeGroupValidator{}

type nodeGroupValidator struct {
	client.Client
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *nodeGroupValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	o := obj.(*NodeGroup)
	nodegrouplog.Info("validating create", "name", o.Name)
	if err := o.Spec.Validate(); err != nil {
		return nil, err
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *nodeGroupValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	o := oldObj.(*NodeGroup)
	n := newObj.(*NodeGroup)
	nodegrouplog.Info("validating update", "name", o.Name)
	if err := n.Spec.Validate(); err != nil {
		return nil, err
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *nodeGroupValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	o := obj.(*NodeGroup)
	nodegrouplog.Info("validating delete", "name", o.Name)
	return nil, nil
}
