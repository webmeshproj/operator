/*
Copyright 2023 Avi Zimmerman <avi.zimmerman@gmail.com>

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

// Package resources contains Kubernetes resource definitions.
package resources

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/webmeshproj/operator/api/v1"
)

// Apply applies the given resources to the cluster.
func Apply(ctx context.Context, cli client.Client, resources []client.Object) error {
	for _, obj := range resources {
		log.FromContext(ctx).Info("Applying object", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
		if err := cli.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner(v1.FieldOwner)); err != nil {
			return fmt.Errorf("failed to apply %s/%s/%s: %w",
				obj.GetObjectKind().GroupVersionKind().Kind,
				obj.GetNamespace(),
				obj.GetName(),
				err,
			)
		}
	}
	return nil
}

// Pointer returns a pointer to the given value.
func Pointer[T any](v T) *T {
	return &v
}
