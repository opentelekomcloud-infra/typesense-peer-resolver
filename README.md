🚨🚨🚨**Repository Archived** 📦

This repository has been archived and is no longer maintained. It has been replaced by [Typesense Kubernetes Operator](https://github.com/akyriako/typesense-operator), which will receive all future updates and improvements.
Please stop using this Helm Chart and refer to the new repository. Thank you!

# Typesense Peer Resolver for Kubernetes

A sidecar container for Typesense that automatically reset the nodes peer value for HA Typesense clusters in Kubernetes
by identifying the new endpoints of the headless service.

### Problem
When restarting/upgrading Typesense nodes in a high-availability cluster scenario running in Kubernetes, 
DNS entries of the StatefulSet do not get resolved again with the new IP, causing the pod to be unable to rejoin the cluster,
even if you have enabled the `TYPESENSE_RESET_PEERS_ON_ERROR` flag

### Solution
Instead of storing the `nodeslist` values in a configmap (as a file through a volume), the `nodeslist` volume is configured 
with `emptyDir` and the sidecar container dynamically updates the values of the nodelist. To do this it watches the endpoints 
in the configured namespace for changes, and sets the collected IPs as node values rather than using the internal DNS name of the Pod. 

```
typesense-0.ts.typesense.svc.cluster.local:8107:8108
```

> [!NOTE]
> Entries in node list, according to the documentation, have to adhere the following pattern: 
> `statefulSetName-0.<headless-svc>.<namespace>.svc.cluster.local:8107,8108`

but in this case the DNS entries will be replaced by the ephemeral IPs of the Pods: `10.244.1.215:8107:8108`

## Context

Normally you'd have a `ConfigMap` like this

```
apiVersion: v1
kind: ConfigMap
metadata:
  name: nodeslist
  namespace: typesense
data:
  nodes: "typesense-0.ts.typesense.svc.cluster.local:8107:8108,typesense-1.ts.typesense.svc.cluster.local:8107:8108,typesense-2.ts.typesense.svc.cluster.local:8107:8108"
```

which will be loaded as an `env` variable in the `StatefulSet` that will be facilitated by a `VolumeMount` that will load
the `ConfigMap` data as a file in the filesystem of the `Pod`: (parts of the manifests have been removed for brevity)

```
...
    
    - name: TYPESENSE_NODES
      value: "/usr/share/typesense/nodes"
    ...

    volumeMounts:
        - name: nodeslist
            mountPath: /usr/share/typesense
        - name: data
            mountPath: /usr/share/typesense/data
...

volumes:
    - name: nodeslist
        configMap:
        name: nodeslist
        items:
            - key: nodes
            path: nodes
```

## Usage & Configuration

### Change Volume to `emptyDir`

1. You can discard the `configMap` entirely, unless you use it for other values. N

> [!CAUTION]
> Use a `Secret` for the API keys or other sensitive values.

2. Leave the `volumeMounts` as is.

3. Replace the `volumes`, we discussed above, with the following:

```
volumes:
    - name: nodeslist
        emptyDir: {}
```

### Create RBAC

In order the watcher to be able to get a list of the endpoints, it has to be granted the permissions to the specific resources.
For that matter we will create in the same namespace we installed Typesense a `ServiceAccount`, a `Role` and a `RoleBinding`:

```
apiVersion: v1
kind: ServiceAccount
metadata:
  name: typesense-service-account
  namespace: typesense
```

```
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: typesense-role
  namespace: typesense
rules:
- apiGroups: [""]
  resources: ["endpoints"]
  verbs: ["get", "watch", "list"]
```

```
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: typesense-role-binding
  namespace: typesense
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: typesense-role
subjects:
- kind: ServiceAccount
  name: typesense-service-account
  namespace: typesense
```

### Assign the `ServiceAccount` to the `Pod` of the `StatefulSet`

In order to bind the containers running in the pod with the service account we just created, add the `serviceAccountName`
property in the manifest of the `StatefulSet`:

```yaml
spec:
      containers:
        - name: typesense
          image: typesense/typesense:26.0
          ...
      serviceAccountName: typesense-service-account
      securityContext:
        fsGroup: 2000
        runAsGroup: 3000
        runAsNonRoot: true
        runAsUser: 10000
```

### Add the sidecar

Last step is to add the sidecar definition in the manifest of `StatefulSet` (under the `containers` stanza):

```yaml
- name: peer-resolver
  image: akyriako78/typesense-peer-resolver:latest
  command:
    - "/opt/tspr"
    - "-namespace=typesense"
    - "-service=typesense-svc"
  volumeMounts:
    - name: nodeslist
      mountPath: /usr/share/typesense
```

> [!NOTE]
> You can of course build and use your own container image:
> 
> ```shell
> docker build . -t <docker-account>/typesense-peer-resolver:<tag>
> docker push <docker-account>/typesense-peer-resolver:<tag>
> ```

## Develop & Test Locally

You can of course work outside of the cluster, by running or debugging the code from your IDE of preference. The tool has
the following command arguments:

* `-kubeconfig (default=config)`: kubeconfig file in ~/.kube to work with
* `-namespace (default=typesense)`: namespace that typesense is installed within
* `-service (default=typesense-svc)`: name of the typesense service to use the endpoints of
* `-nodes-file (default=/usr/share/typesense/nodes)`: location of the file to write node information to
* `-peer-port (default=8107)`: port on which typesense peering service listens
* `-api-port (default=8108)`: port on which typesense API service listens

> [!IMPORTANT]
> **Major difference** from the upstream is that this version can identify even the IP addresses of **non-ready** Typesense pods,
> in case you cannot or don't want to enable the `publishNotReadyAddresses` property of the headless service. That combination could
> lead to a _Catch22 situation_ where the pods could not get in _Ready_ state because they do not have a nodes-list defined and 
> the `s.Addresses` is always null as the headless service is not publishing the endpoints of not-ready pods.  
>
> ```go
>    for _, s := range e.Subsets {
>		addresses := s.Addresses
>		if s.Addresses == nil || len(s.Addresses) == 0 {
>			addresses = s.NotReadyAddresses
>		}
>		for _, a := range addresses {
>			for _, p := range s.Ports {
>				if int(p.Port) == apiPort {
>					nodes = append(nodes, fmt.Sprintf("%s:%d:%d", a.IP, peerPort, p.Port))
>				}
>			}
>		}
>	}
> ```
> with the change above is guaranteed that the watcher will collect the endpoints of the new peers and add the to the nodes list.

You can find a complete ready-to-install manifest here: [typesense.yaml](/typesense.yml)

