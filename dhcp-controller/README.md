
# dhcp-controller

This is primarily for kubevirt VM's, but intention is to have  a generic solution which can be used by pods as well.

### Issue with CNI approach

IPAM CNI (whereabouts, host-local) are used as delegate from backend CNI which gets managed/triggered at pod creation and  pod deletion.
This has issues with VM migration scenario. VM migration involves deletion of virt-launcher pod at source host and creation of new virt-launcher pod at destination host

### Alternative 


To have a DHCP server running inside pod/vm to cater to the DHCP requests from virtual machine instance(not pod in case of Kubevirt).
Multus net-attac-def planning to use dhcp server, need not to specify IPAM plugin in there. Client/Consumer VM will need dhclient for sendingd request.

This uses dnsmasq to cater the client request. Etcd is used for storing the allocation.

### Schema for creating a DHCPServer

    // DHCPServerSpec defines the desired state of DHCPServer
    type DHCPServerSpec struct {
    	// Details of Servers
    	Servers []Server `json:"servers,omitempty"`
    }
    type Server struct {
    	// refers to net-attach-def to be served
    	NetworkName string `json:"networkName,omitempty"`
    	// refers to IP address to bind interface to
    	InterfaceIp string `json:"interfaceIp,omitempty"`
    	// refers to CIDR of server
    	ServerCIDR CIDR `json:"cidr"`
    	// refers to leasetime of IP
    	LeaseTime string `json:"leaseTime,omitempty"`
    	// refers to vlan
    	VlanID string `json:"vlanid,omitempty"`
    }
    // CIDR defines CIDR of each network
    type CIDR struct {
    	// refers to cidr range
    	CIDRIP string `json:"range"`
    	// refers to start IP of range
    	RangeStartIp string `json:"range_start,omitempty"`
    	// refers to end IP of range
    	RangeEndIp string `json:"range_end,omitempty"`
    	// refers to gateway IP
    	GwAddress string `json:"gateway,omitempty"`
    }

### Schema for IPAllocation used by controller to store/manage the Allocation 

    // IPAllocationSpec defines the desired state of IPAllocation
    type IPAllocationSpec struct {
    	// Range is a string that represents an IP address
    	Range string `json:"range"`
    	// Allocations is the set of allocated IPs for the given range. Its` indices are a direct mapping to the
    	// IP with the same index/offset for the allocation's range.
    	Allocations map[string]IPAllocationOwner `json:"allocations"`
    	// EpochExpiry is the epoch time when the IP was set to expire in the leasefile
    	EpochExpiry string `json:"epochexpiry"`
    }
    // IPAllocationOwner represents metadata about the pod/container owner of a specific IP
    type IPAllocationOwner struct {
    	MacAddr string `json:"macaddr,omitempty"`
    	VmiRef  string `json:"vmiref,omitempty"`
    }


### Sample yamls to work with.


    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: dhcpserver-sample
      namespace: default
    data:
      dnsmasq.conf: |
        port=0
        dhcp-range=192.168.15.10,192.168.15.100,255.255.255.0,0h
        dhcp-option=3,192.168.15.1
    ---
    apiVersion: dhcp.plumber.k8s.pf9.io/v1alpha1 
    kind: DHCPServer
    metadata:
      name: dhcpserver-sample
    spec:
      servers:
        - networkName: ovs-dnsmasq-test
          interfaceIp: 192.168.15.54
          leaseTime: 10m
          vlanid: vlan1
          cidr:
            range: 192.168.15.0/24
            range_start: 192.168.15.10
            range_end: 192.168.15.100
            gateway: 192.168.15.1
        - networkName: ovs-build-pmk-provider-net
          interfaceIp: 10.128.144.90
          leaseTime: 10m
          vlanid: vlan2
          cidr:
            range: 10.128.144.0/23
            range_start: 10.128.144.10
            range_end: 10.128.145.200

**Note**: Providing the configmap is optional. If not provided, one will automatically be generated with the needed configurations. If any custom parameters are needed to be set, create a configmap with valid dnsmasq.conf parameters. Along with this, ```dhcp-range``` must be in one of the two formats
1. ```dhcp-range=<start_IP>,<end_ip>,<netmask>,<leasetime>```
2. ```dhcp-range=<vlanID>,<start_ip>,<end_ip>,<netmask>,<leasetime>```

## Working

* Using the luigi addons, DHCPController is created in dhcp-controller-system namespace.
* When DHCPServer is created, a deployment is made. It creates a DHCPServer pod, which runs with dnsmasq.
* A configmap is generated based on the DHCPServer. This is a conf file for dnsmasq. It can be overridden by creating a valid configmap with the same name as that of the DHCPServer.
* Changes to the configmap will cause redeployment of the DHCPServer.
* The leasefile is watched for changes (write events). Accordingly, IPAllocation objects are created/deleted/updated. Epoch time of the lease is also stored.
* If a VM live-migrates, all IPs allocated to the secondary interfaces are retained, provided that the MacAddresses of the NICs stay the same
* If a DHCPServer is redeployed, leases that belong to the network(s) that the DHCPServer is serving are restored with the same lease time from the IPAllocations.



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

