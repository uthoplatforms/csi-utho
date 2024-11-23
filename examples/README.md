## Installation

### Requirements

A Utho cluster

### Kubernetes secret

In order for the csi to work properly, you will need to deploy a
[kubernetes secret](https://kubernetes.io/docs/concepts/configuration/secret/).
To obtain a API key, please visit
[API settings](https://console.utho.com/api).

The `secret.yml` definition is as follows. You can also find a copy of this yaml
[here](secret.yml).

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: csi-utho
  namespace: kube-system
stringData:
  # Replace the api-key with a proper value
  api-key: <API_KEY>
```

To create this `secret.yml`, you must run the following

```sh
kubectl create -f examples/secret.yaml
secret/utho-csi created
```

### Deploying the CSI

To deploy the latest release of the CSI to your Kubernetes cluster, run the
following:

`kubectl apply -f deploy/latest.yml`

### Validating

The deployment will create a
[Storage Class](https://kubernetes.io/docs/concepts/storage/storage-classes/)
which will be used to create your volumes

```sh
kubectl get storageclass
NAME                        PROVISIONER    RECLAIMPOLICY   VOLUMEBINDINGMODE   ALLOWVOLUMEEXPANSION   AGE
utho-block-storage-retain   csi.utho.com   Delete          Immediate           false                  131m
```

To further validate the CSI, create a
[PersistentVolumeClaim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)

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

Now, take the yaml shown above and create a `pvc.yml` and run:

`kubectl create -f examples/pvc.yml`

You can then check that you have a unattached volume on the Utho dashboard. In
addition, you can see that you have a `PersistentVolume` created by your Claim

```sh
kubectl get pv
NAME                   CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM          STORAGECLASS          AGE
pvc-2579a832202d4d07   10Gi       RWO            Delete           Bound    csi-utho-pvc   utho-block-storage    2s
```

Again, this volume is not attached to any node/pod yet. The volume will be
attached to a node when a pod residing inside that node requests the specific
volume.

Here is an example yaml of a pod request for the volume we just created.

```yaml
kind: Pod
apiVersion: v1
metadata:
  name: csi-app
spec:
  containers:
    - name: csi-app
      image: busybox
      command: [ "sleep", "1000000" ]
      volumeMounts:
        - mountPath: "/data"
          name: myvolume
  volumes:
    - name: myvolume
      persistentVolumeClaim:
        claimName: csi-utho-pvc
```

`kubectl create -f examples/pod-volume.yml`

To get more information about the pod to ensure it is running and mounted, you
can run the following

`kubectl describe po csi-app`

Now, let's add some data to the pod and validate that if we delete a pod and
recreate a new pod which requests the same volume, the data still exists.

```sh
# Create a file
kubectl exec -it csi-app -- /bin/sh -c "touch /data/example"

# Delete the Pod
kubectl delete -f examples/pod-volume.yml

# Recreate the pod with the same volume
kubectl create -f examples/pod-volume.yml

# See that data on our volume still exists
kubectl exec -it csi-app -- /bin/sh -c "ls /data"
```
