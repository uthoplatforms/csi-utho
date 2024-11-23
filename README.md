# csi-utho

A Container Storage Interface ([CSI](https://github.com/container-storage-interface/spec)) Driver for Utho Block Storage. The CSI plugin allows you to use Utho Block Storage with your preferred Container Orchestrator.


## Releases

The Utho CSI plugin follows [semantic versioning](https://semver.org/).
The version will be bumped following the rules below:

* Bug fixes will be released as a `PATCH` update.
* New features (such as CSI spec bumps with no breaking changes) will be released as a `MINOR` update.
* Significant breaking changes makes a `MAJOR` update.

## Features

Below is a list of functionality implemented by the plugin. In general, [CSI features](https://kubernetes-csi.github.io/docs/features.html) implementing an aspect of the [specification](https://github.com/container-storage-interface/spec/blob/master/spec.md) are available on any Utho Kubernetes version for which beta support for the feature is provided.

See also the [project examples](/examples) for use cases.

## Installing to Kubernetes

### Kubernetes Compatibility

The following table describes the required Utho CSI driver version per supported Kubernetes release.

| Kubernetes Release | Utho CSI Driver Version |
|--------------------|---------------------------------|
| 1.30               | v1.0.0+                         |
| 1.31               | v1.0.1+                         |

---
**Note:**

The [Utho Kubernetes](https://utho.com/kubernetes) product comes with the CSI driver pre-installed and no further steps are required.

---

#### 1. Create a secret with your Utho API Access Token

Replace the placeholder string with `API_KEY` with your own secret and
save it as `secret.yml`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: csi-utho
  namespace: kube-system
stringData:
  api-key: <API_KEY>
```

and create the secret using kubectl:

```shell
$ kubectl create -f ./secret.yml
secret "csi-utho" created
```

You should now see the utho secret in the `kube-system` namespace along with other secrets


#### 3. Deploy the CSI plugin and sidecars

Always use the [latest release](https://github.com/utho/csi-utho/releases) compatible with your Kubernetes release (see the [compatibility information](#kubernetes-compatibility)).

The [releases directory](deploy/kubernetes/releases) holds manifests for all plugin releases. You can deploy a specific version by executing the command

```shell
# Do *not* add a blank space after -f
kubectl apply -f deploy/latest.yml
```

#### 4. Test and verify

Create a PersistentVolumeClaim. This makes sure a volume is created and provisioned on your behalf:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-utho-pvc
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: utho-block-storage
  resources:
    requests:
      storage: 10Gi
```

Check that a new `PersistentVolume` is created based on your claim:

```shell
$ kubectl get pv
```

The volume is not attached to any node yet. It'll only attached to a node if a
workload (i.e: pod) is scheduled to a specific node. Now let us create a Pod
that refers to the above volume. When the Pod is created, the volume will be
attached, formatted and mounted to the specified Container:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: csi-utho-test-deployment
spec:
  replicas: 1
  selector:
    matchLabels:
      app: csi-utho-test
  template:
    metadata:
      labels:
        app: csi-utho-test
    spec:
      containers:
        - name: app
          image: busybox
          command: ["sleep", "3600"]
          volumeMounts:
            - mountPath: "/data"
              name: myvolume
      volumes:
        - name: myvolume
          persistentVolumeClaim:
            claimName: csi-utho-pvc
```

Check if the pod is running successfully:

```shell
kubectl get deployment csi-utho-test-deployment
```