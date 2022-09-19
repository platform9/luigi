# dhcp-controller

This is primarily for kubevirt VM's, but intention is to have  a generic solution which can be used by pods as well.

#Issue with CNI approach

IPAM CNI (whereabouts, host-local) are used as delegate from backend CNI which gets managed/triggered at pod creation and  pod deletion.
This has issues with VM migration scenario. VM migration involves deletion of virt-launcher pod at source host and creation of new virt-launcher pod at destination host

#Alternative 


To have a DHCP server running inside pod/vm to cater to the DHCP requests from virtual machine instance(not pod in case of Kubevirt).
Multus net-attac-def planning to use dhcp server, need not to specify IPAM plugin in there. Client/Consumer VM will need dhclient for sendingd request.

This uses dnsmasq to cater the client request. Etcd is used for storing the allocation.

#Schema for creating a DHCPServer
// DHCPServerSpec defines the desired state of DHCPServer
type DHCPServerSpec struct {

        // refers to net-attach-def to be served
        // +kubebuilder:validation:Required
        NetworkName string `json:"networkName,omitempty"`
        // refers to IP address to be configured at BindInterface port
        // +kubebuilder:validation:Required
        InterfaceIp string `json:"interfaceIp,omitempty"`
	// configmap to be mounted as dnsmasq.conf
        // +kubebuilder:validation:Required
        ConfigMapName string `json:"configMapName,omitempty"`
}

#Schema for Ippools used by controller to store/manage the Allocation 
// IPAllocationSpec defines the desired state of IPAllocation
type IPAllocationSpec struct {
        // INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
        // Important: Run "make" to regenerate code after modifying this file
        // Range is a RFC 4632/4291-style string that represents an IP address and prefix length in CIDR notation
        Range string `json:"range"`
        // Allocations is the set of allocated IPs for the given range. Its` indices are a direct mapping to the
        // IP with the same index/offset for the allocation's range.
        Allocations map[string]IPAllocationOwner `json:"allocations"`
}

// IPAllocationOwner represents metadata about the pod/container owner of a specific IP
type IPAllocationOwner struct {
        MacId  string `json:"id,omitempty"`
        VmiRef string `json:"vmiref,omitempty"`
}


#Sample yamls to work with.

"""
apiVersion: v1
kind: ConfigMap
metadata:
  name: dhcp1-config
  namespace: default
data:
  dnsmasq.conf: |
    port=0
    dhcp-range=192.168.15.10,192.168.15.100,255.255.255.0,0h
    dhcp-option=3,192.168.15.1
"""
apiVersion: dhcp.plumber.k8s.pf9.io/v1alpha1 
kind: DHCPServer
metadata:
  name: dhcpserver-sample
spec:
  networkName: ovs-ipam-vm-trunk-net //Multus network-attachment-definition
  interfaceIp: 192.168.15.9
  configMapName: dhcp1-config
"""




## Getting Started
You will need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster
1. Install Instances of Custom Resources:

```sh
   make install
   cd dhcpserver && make install
```

2. Build and push your image to the location specified by `IMG`:
	
```sh
make docker-build docker-push IMG=<some-registry>/dhcp-controller:tag
cd dhcpserver && make docker-build docker-push IMG=<some-registry>/dnsmasq:tag
```
	
3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/dhcp-controller:tag
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller to the cluster:

```sh
make undeploy
```

## Contributing

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) 
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster 

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

