# Webmesh Operator

The repository contains an operator running [Webmesh nodes](https://github.com/webmeshproj/node) on Kubernetes.

## Getting started

You must have `cert-manager` installed first.
This is used to generate TLS certificates for the mesh.
You can install it by running:

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.yaml
```

You can deploy the operator to an existing repository by cloning this repository and running:

```bash
make deploy
```

You can also use the `kustomize` manifests in the `config` directory to deploy the operator to an existing cluster.

### Using K3d

The `Makefile` contains helpers for doing the same locally via a `k3d` cluster.
It should work the same on a `kind` cluster. But you'll need a load balancer (e.g. metallb) to expose the nodes.

To setup a `k3d` cluster run:

```bash
# You can skip the docker-build step if you have the image pulled locally
make docker-build run-k3d
```

Once the cluster is ready, you can install the operator and CRDs to the local cluster by running:

```bash
make install-operator
```

By default the operator will run in the `webmesh-system` namespace:

```bash
$ kubectl get pod -n webmesh-system
NAME                                          READY   STATUS    RESTARTS   AGE
operator-controller-manager-67cc849c6-7nlbk   1/1     Running   0          55s
```

Example manifests can be found in the [`config/samples`](config/samples/) directory.
To bootstrap a new mesh, you can create a `WebMesh` resource:

```bash
$ kubectl apply -f config/samples/mesh_v1_mesh.yaml
mesh.mesh.webmesh.io/mesh-sample created
```

When the mesh is ready, you should have 4 pods running.
Three bootstrap nodes and a load-balancer node that exposes the mesh to the outside world:

```bash
$ kubectl get pod
NAME                         READY   STATUS    RESTARTS   AGE
mesh-sample-bootstrap-0      1/1     Running   0          46s
mesh-sample-bootstrap-1      1/1     Running   0          46s
mesh-sample-bootstrap-2      1/1     Running   0          46s
mesh-sample-bootstrap-lb-0   1/1     Running   0          41s
```

An admin configuration for the wmctl utility in the [node repository](https://github.com/webmeshproj/node) is written to a secret.
You can retrieve it by running:

```bash
make get-config
```

## Building

This is just your typical `kubebuilder` project.
You can build the operator by running:

```bash
make docker-build
```

Run `make help` to see all the available targets.

## Contributing

Contributions are welcome.
Please feel free to open an issue or a pull request.
One thing I'd like to get done in the short-term is support for creating nodes across all the major cloud providers.
Currently only GCP Compute Instances are supported.
