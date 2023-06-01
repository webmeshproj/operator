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
	"errors"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
			config: mgr.GetConfig(),
		}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-mesh-webmesh-io-v1-nodegroup,mutating=true,failurePolicy=fail,sideEffects=None,groups=mesh.webmesh.io,resources=nodegroups,verbs=create;update,versions=v1,name=mnodegroup.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &NodeGroup{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *NodeGroup) Default() {
	nodegrouplog.Info("defaulting", "name", r.Name)

	// Ensure a default config
	if r.Spec.ConfigGroup == "" && r.Spec.Config == nil {
		r.Spec.Config = &NodeGroupConfig{}
		r.Spec.Config.Default()
	} else if r.Spec.Config != nil {
		r.Spec.Config.Default()
	}

	// TODO: Handle non-cluster node groups
	if r.Spec.Cluster == nil {
		r.Spec.Cluster = &NodeGroupClusterConfig{}
	}
	r.Spec.Cluster.Default()
}

//+kubebuilder:webhook:path=/validate-mesh-webmesh-io-v1-nodegroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=mesh.webmesh.io,resources=nodegroups,verbs=create;update,versions=v1,name=vnodegroup.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &nodeGroupValidator{}

type nodeGroupValidator struct {
	client.Client
	config  *rest.Config
	localID string
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *nodeGroupValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	o := obj.(*NodeGroup)
	nodegrouplog.Info("validating create", "name", o.Name)
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *nodeGroupValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	o := oldObj.(*NodeGroup)
	nodegrouplog.Info("validating update", "name", o.Name)
	if val, ok := o.GetAnnotations()[BootstrapNodeGroupAnnotation]; ok && val == "true" {
		// Bootstrap group can only be mutated by the controller
		if r.localID == "" {
			// Hit token endpoint to get local ID
			cli, err := kubernetes.NewForConfig(r.config)
			if err != nil {
				return nil, err
			}
			res, err := cli.AuthenticationV1().TokenReviews().Create(ctx, &authv1.TokenReview{
				Spec: authv1.TokenReviewSpec{
					Token: r.config.BearerToken,
				},
			}, metav1.CreateOptions{})
			if err != nil {
				return nil, err
			}
			if !res.Status.Authenticated {
				return nil, errors.New("unable to authenticate with API server")
			}
			r.localID = res.Status.User.UID
		}
		req, err := admission.RequestFromContext(ctx)
		if err != nil {
			return nil, err
		}
		if req.UserInfo.UID != r.localID {
			return nil, errors.New("bootstrap node groups can only be mutated by the Mesh controller")
		}
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *nodeGroupValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	o := obj.(*NodeGroup)
	nodegrouplog.Info("validating delete", "name", o.Name)
	return nil, nil
}
