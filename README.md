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

1) You can discard the `configMap` entirely, unless you use it for other values. Normally you're using a `secret` for the API key anyways.

> [!CAUTION]
> Entries in node list, according to the documentation, have to adhere the following pattern:
> `statefulSetName-0.<headless-svc>.<namespace>.svc.cluster.local:8107,8108`

2) Leave the `volumeMounts` as they were.

3) Replace the `volumes` we discussed above with the following to the following:

```
volumes:
    - name: nodeslist
        emptyDir: {}
```

4) You'll need to create a `ServiceAccount` , `Role` and `RoleBinding` for the sidecar.

```
apiVersion: v1
kind: ServiceAccount
metadata:
  name: typesense-service-account
  namespace: typesense
# imagePullSecrets:
#   - name: your-image-pull-secret
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
  verbs: ["watch", "list"]
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

5) Finally, you can add the sidecar to the pod containers and set one of a handful of configuration parameters

```
- name: ts-node-resolver
    image: alasano/typesense-node-resolver
    command:
        - "/opt/tsns"
        - "-namespace=someOtherNamespace"
    volumeMounts:
        - name: nodeslist
        mountPath: /usr/share/typesense

```

* `-namespace=NS` // _Namespace in which Typesense is installed (default: typesense)_
* `-service=SVC` // _Service for which to retrieve endpoints (default: ts)_
* `-nodes-file=PATH` // _The location to write the nodes list to (default: /usr/share/typesense/nodes)
* `-peer-port=PORT` // _Port on which Typesense peering service listens (default: 8107)_
* `-api-port=PORT` // _Port on which Typesense API service listens (default: 8108)_

### Full Example

You can see a full example in [typesense.yaml](/typesense.yml)


_All credit for initial implementation goes to [Elliot Wright](https://github.com/seeruk) - Forked from [github.com/seeruk/tsns](https://github.com/seeruk/tsns)_