# port-forward

Port Forward is a Kubernetes Controller that forwards external ports to Kubernetes Services of type LoadBalancer that are assigned private IP addresses. This is useful for clusters using something like MetalLB to expose Services internally that want to expose some of them externally.

## install

```sh
kubectl kustomize https://github.com/frantjc/port-forward/config/default?ref=v0.1.4 | kubectl apply -f-
```

## use

```sh
kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: sample
  annotations:
    pf.frantj.cc/forward: "yes"
spec:
  type: LoadBalancer
  ports:
    - port: 443
      targetPort: 443
  selector: {}
EOF
```

> See [sample](./config/samples/service.yaml) for full list of supported annotations.

## developing

You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.

Port Forward will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### how it works

Uses the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

Uses a [Controller](https://kubernetes.io/docs/concepts/architecture/controller/), which provides a reconcile function responsible for synchronizing Services of type LoadBalancer until the desired state is reached and maintained on the cluster.

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html).

Uses SNAT and UPnP to tell a router what to port forward. Written in such a way that more secure implementations can be written for networking devices that support them such as OPNsense which has an API to do such things.
