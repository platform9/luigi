# yoshi
Yoshi is a Network and VM controller/webhook to simplify Kubevirt VM operations. 

It is composed of two parts:

## NetworkWizard
NetworkWizard is a new CRD/API to create an onionated network, provided by a backend CNI plugin. It's goal is to hide the various CRs and advanced configuration options, and allow configuration of everything via one simple(r) CR.

For example with Calico, it will automatically create the necessary IPPools and BGP configuration.

Support is planned for L2 Multus network creations. For example with the OVS CNI, it will create the needed Multus NetworkAttachment Definitions, configure the nodes with Hostplumber to create the OVS bridges, and deploy the PF9 DHCP Server.

For advanced usecases or power users, it is not recommended to use this feature - instead use the direct Calico, Multus, Hostplumber, DHCP, etc.. templates. For example, if you care about VLANs, Calico's IPinIP vs VXLAN, customizing BGP prefix advertisements, then this is probably not for you. However if these terms went over your head and you "just want a network", then this is probably for you.

### Creating a Calico network

```
apiVersion: plumber.k8s.pf9.io/v1
kind: NetworkWizard
metadata:
  name: newarjunpool
spec:
  plugin: "calico"
  cidr: "10.133.0.0/16"
```

Above will create a new Calico network with the CIDR 10.33.0.0/16. VXLAN will be enabled for east-west traffic and providing isolation across nodes/TORs. Northbound traffic will be SNAT'd using the node's IP.

### Creating an OVS Multus network
TODO: Support for OVS/Multus is not implemented yet, but is planned.

### Creating a public network

A public network is for providing VMs with public IPs. The VM will still get an internal IP on the K8s CNI-backed network. This orchestrates configuration of BGP via Calico and MetalLB in order to provide ExternalIPs and External LoadBalancer IPs.

```
apiVersion: plumber.k8s.pf9.io/v1
kind: NetworkWizard
metadata:
  name: publicnetwork
spec:
  plugin: "public"
  cidr: "207.97.241.196/29"
  bgpConfig:
    peers: 10.240.0.5, 10.240.0.4
    remoteASN: 64512
    myASN: 64514
```

This will create a Public pool comprised of the CIDR 207.97.241.196/29. If the BGPConfig is missing, it will use MetalLB in L2 mode - which means ARP/GARP to announce external Loadbalancer IPs and requires the public network to be directly reachable via L2 from the nodes. If BGPConfig is present, it will configure a cluster-wide BGP session via pure L3. Once again, if you wish to do something more complex then it's recommended to directly use the MetalLB and Calico resources directly.

### VM Fixed IP (Calico)
In the K8s networking model, "ports" are not a first class resource. Furthermore, fixed IPs are not a common or supported concept. "Cloud-native" "workloads" aka Pods can have their IP change if recreated. In fact, even if the container dies and is recreated, the Pod will come up with a new IP. If a VM is rescheduled to evicted to a new node, it will get a new IP. This aims to solve that.

By default, Calico IPAM allocates a unique cidr to each node, carved out of the larger subnet. However, it is possible to assign a fixed IP. But the onus should not be on the user to do fixed IP IPAM. This aims to solve that.

Annotate your Kubevirt VM with `plumber.k8s.pf9.io/networkName: <networkWizardName>"

```
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: vmcalicobridge2
  annotations:
    'plumber.k8s.pf9.io/networkName': 'newarjunpool'
```

The VM will automatically get a Fixed IP for it's lifecycle.

The network name must match the above NetworkWizard CR name. Yoshi will provide a fixed IP IPAM layer on top of Calico's IPAM that will allocate a unique IP, then annotate the VM with the assigned IP. Further more, since Calico apparently does not have a way to via IP allocations - even for Pods - Yoshi will append the matching NetworkWizard's Status with a list of IP Allocations to VM name.

### VM Public IP

1. Annotate the VM with a Public IP. This assumes you already have a Public/Elastic IP assigned by your cloud provider

```
annotations:
  "plumber.k8s.pf9.io/publicIP": "1.2.3.4"
```

The Yoshi controller will create a Service resource for the VM, set an external Loadbalancer IP, then set the required BGP Advertisements to announce this IP from the node of the VM. This will be advertised as a /32 route, regardless of the CIDR in the NetworkWizard's "public" network. 

Note that this means if using an epxlicit IP in the VM annotation, the NetworkWizard public network does not requir a CIDR.

2. Annotate the VM with a Public network (must match NetworkWizard)

```
annotations:
  "plumber.k8s.pf9.io/publicNetwork": "yoshiPublicNetwork"
```

The Yoshi controller will auto-allocate a Public IP from the CIDR specified in the "yoshiPublicNetwork" NetworkWizard CR. If using the publicNetwork annotation, it is required the NetworkWizard have a CIDR specified.

** This is not guaranteed to work in a shared cloud environment, as unique IP allocations will only be per-cluster. If thats the case, request a public IP from the provider and explicitly annotate the VM as above**

## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).


