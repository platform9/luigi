# HostPlumber
HostPlumber is a K8s operator based solution to configure your nodes for advanced networking usecases and view each node's networking information.

It defines two new CRDs called HostNetworkTemplate and HostNetwork, used to configure the host’s networking and view node networking state. 

Runs as a Daemonset on each node, responsible for discovering the host’s initial state and populating the HostNetwork CRD, watching the HostNetworkTemplate CRD and determining whether to apply the config template to the host, then reporting the new state to the HostNetwork CRD. It will also periodically check, and enforce the networking configuration has not deviated on each host.

-   Requires privileged securityContext

## What it can do:

 - Configuring SRIOV VFs and drivers
 - Creating VLAN interfaces
 - Creating OVS bridges and adding interfaces
 - Device MTUs
 - IP addresses and Routes
 - And more planned for the future!


## How to deploy

For now, the easiest way to deploy is via the Luigi operator's NetworkPlugins CRD. Please see: https://github.com/platform9/luigi

## How to use

By default, a HostNetworkTemplate spec’s will be applied globally on each host, best effort. Or with the use of Labels and Node selectors, allowing to selectively target a one node or a subset of Nodes.

It is best to demonstrate by example. Here is a comprehensive HostNetworkTemplate:

    apiVersion: plumber.k8s.pf9.io/v1
    kind: HostNetworkTemplate
    metadata:
      name: hostconfig-kernel-eno2
    spec:
      # Add fields here
      nodeSelector:
        feature.node.kubernetes.io/network-sriov.capable: "true"
        foo: "bar"
      sriovConfig:
        - pfName: eno2
          numVfs: 4
          vfDriver: i40evf
      interfaceConfig:
        - name: eno2
          mtu: 9000
          vlan:
            - id: 1000
            - id: 1001
            - id: 1002
      ovsConfig:
      - bridgeName: ovs-br01
        nodeInterface: eno2.1000


This example does a few things:
1. The template will only be applied on nodes with the label network-sriov.capable = "true" *(How is this added? Automatically via node-feature-discovery plugin, also installed by Luigi!)*, AND, with the label foo="bar". If we remove the nodeSelector section, the template would be applied on every node in the cluster
2. It creates 4 SRIOV VFs, on the NIC eno2, using the i40evf driver
3. It creates 3 VLAN interfaces: eno2.1000, eno2.1001, eno2.1002
4. It creates an OVS bridge named ovs-br01, and adds the interface eno2.1000 to the bridge.

Of course, it is not necessary to place all configurations in one file. You can create as many HostNetworkTemplate CRDs as you want for different groups of nodes, separate NICs

## sriovConfig
The sriovConfig section takes a list of device filters, the number of VFs to configure on each device, and the driver to use:

 - **numVfs**: Integer specifying how many VFs to create under the device
 - **vfDriver**: The VF driver to use and load - i40evf, ixgbevf for
   example. For DPDK/Kubevirt, vfio-pci is typically used
   
**The actual device(s) can be filtered in ONE of several ways:**
 - **pfName** - Name of the NIC, aka Physical Function
 ```
apiVersion: plumber.k8s.pf9.io/v1
kind: HostNetworkTemplate
metadata:
  name: sriovconfig-enp3s0f1
spec:
  # Add fields here
  sriovConfig:
    - pfName: enp3s0f1
      numVfs: 8
      vfDriver: ixgbevf
---
apiVersion: plumber.k8s.pf9.io/v1
kind: HostNetworkTemplate
metadata:
  name: sriovconfig-enp3s0f0
spec:
  # Add fields here
  sriovConfig:
    - pfName: enp3s0f0
      numVfs: 4
      vfDriver: vfio-pci
```

In the above example, we define 2 CRDs, named sriovconfig-enp3s0f1 and sriovconfig-enp3s0f0. It will creates 8 VFs on physical interface enp3s0f1, binds them to the ixgbevf driver. It will also create 4 VFs on interface enp3s0f0 and bind them to the vfio-pci driver

 - **Vendor and Device ID** - useful when you want to apply SRIOV configuration to all types of a NIC, where the naming scheme may not be known

```
apiVersion: plumber.k8s.pf9.io/v1
kind: HostNetworkTemplate
metadata:
  name: HostNetworkTemplate-1528-dev
spec:
  # Add fields here
  sriovConfig:
    - vendorId: "8086"
      deviceId: "1528"
      numVfs: 32
      vfDriver: vfio-pci
```
The above will search for all interfaces matching vendor ID 8086 (Intel) and device ID 1528 (representing a particular model of NIC). It will then create 32 VFs on each matching device and bind all of them to the vfio-pci (DPDK driver). This might be useful if you don’t know the interface naming scheme across your hosts or PCI addresses, but you have the same hardware on all hosts and want to target a particular NIC by vendor and device ID.

 - **PCI Address**
```
apiVersion: plumber.k8s.pf9.io/v1
kind: HostNetworkTemplate
metadata:
  name: HostNetworkTemplate-sample
spec:
  # Add fields here
  sriovConfig:
    - pciAddr: 0000:03:00.0
      numVfs: 32
      vfDriver: vfio-pci
    - pciAddr: 0000:03:00.1
      numVfs: 32
      vfDriver: vfio-pci
```
The above will configure 32 VFs on PF matching PCI address “`0000:03:00.0”` and 32 VFs on PCI address “0000:03.00.1”, for a total of 64 VFs, and bind each VF to the vfio-pci driver.

## interfaceConfig:
The interfaceConfig section can currently be used to configure MTUs, create VLAN interfaces, and configure IP addresses. It takes in a list of interfaces specified by name, with the following options for each:

#### IP addresses
IP address configuration only makes sense if using nodeSelectors to target one, specific node. Addresses reflect final desired state - anything not present will be deleted
```
apiVersion: plumber.k8s.pf9.io/v1
kind: HostNetworkTemplate
metadata:
  name: hostconfig-kernel-enp3
spec:
  # Add fields here
  nodeSelector:
    kubernetes.io/hostname: 10.128.237.204
  interfaceConfig:
    - name: enp3s0f1
      mtu: 9000
      ipv4:
        address: 
          - 192.168.195.7/24
          - 192.168.196.8/24
      ipv6:
        address:
          - fc00:1::3/112
```
In the above, I target a speciifc node, using the hostname as a label, and configure 2 IPv4 addresses, and an IPv6 address on the interface enp3s0f1. It will also set an MTU of 9000 for jumbo frames.

#### VLAN Interfaces
VLAN interfaces may be needed for some network CNIs that do not perform VLAN tagging of their own, such as macvlan. An example was given at the beginning of the combined config, but here is an example that creates only VLAN interfaces:

```
apiVersion: plumber.k8s.pf9.io/v1
kind: HostNetworkTemplate
metadata:
  name: hostconfig-kernel-eno2
spec:
  # Add fields here
  nodeSelector:
    feature.node.kubernetes.io/network-sriov.capable: "true"
  interfaceConfig:
    - name: eno2
      vlan:
        - id: 1000
        - id: 1001
        - id: 1002
    - name: eno1
      vlan:
        - id: 999
```
A list of interfaces is specified, for eno1 and eno2. A vlan interface on 999, and 1000-1002 is created on each, respectively.

# ovsConfig:
This can be used to create OVS bridges and attach interfaces to them. This does NOT deploy OpenVSwitch or install the ovs-vsctl CLI tools for you. Nor does it install the OVS CNI plugin for k8s. To install them, please use the Luigi NetworkPlugins operator, or install these manually.

```
apiVersion: plumber.k8s.pf9.io/v1
kind: HostNetworkTemplate
metadata:
  name: hostconfig-kernel-eno2
spec:
  # Add fields here
  nodeSelector:
    feature.node.kubernetes.io/network-sriov.capable: "true"
  interfaceConfig:
    - name: eno2
      vlan:
        - id: 1000
        - id: 1001
        - id: 1002
  ovsConfig:
  - bridgeName: ovs-br01
    nodeInterface: eno2.1000
```

This will create the OVS bridge with name "ovs-br01", and attach the interface eno2.1000 to it. Please note that eno2.1000 must already exist - the exception is when using HostPlumber like above, to have it created in the same CRD.

The nodeInterface: may be any physical NIC

# HostNetwork CRD:
The HostNetwork CRD will not be created by the user. Instead, this is a read-only CRD and the Daemonset operator on each node will publish various host settings to this CRD:

-   Created: First upon the Daemonset/Operator being deployed
-   Updated: After each application of the HostNetworkTemplate CRD
-   Updated: As a periodic task, every 1 minute

There will be one HostNetwork CRD automatically created for each node, with the same name as the K8s Node name:

```
kubectl get HostNetworks

NAME             AGE
139.178.64.179   2s
145.40.65.95     1s
145.40.78.131    1s
145.40.88.37     1s
147.75.38.91     1s
```

Assume we have created 8 SRIOV VFs, and we inspect node 145.40.65.95:

```
kubectl get hostnetwork 145.40.65.95 -o yaml

apiVersion: plumber.k8s.pf9.io/v1
kind: HostNetwork
metadata:
  creationTimestamp: "2022-01-13T04:15:09Z"
  generation: 1
  managedFields:
  - apiVersion: plumber.k8s.pf9.io/v1
    fieldsType: FieldsV1
    fieldsV1:
      f:spec: {}
      f:status:
        .: {}
        f:interfaceStatus: {}
        f:routes:
          .: {}
          f:ipv4: {}
          f:ipv6: {}
    manager: manager
    operation: Update
    time: "2022-01-13T04:15:09Z"
  name: 145.40.65.95
  namespace: default
  resourceVersion: "753020"
  uid: 808db33e-2338-49fe-aff2-e8c030a59a5e
spec: {}
status:
  interfaceStatus:
  - deviceId: "1572"
    mac: b4:96:91:70:19:38
    mtu: 1500
    pciAddr: "0000:01:00.0"
    pfDriver: i40e
    pfName: eno1
    sriovEnabled: true
    sriovStatus:
      totalVfs: 64
    vendorId: "8086"
  - deviceId: "1572"
    ipv6:
      address:
      - fe80::b696:91ff:fe70:1939/64
    mac: b4:96:91:70:19:39
    mtu: 1500
    pciAddr: "0000:01:00.1"
    pfDriver: i40e
    pfName: eno2
    sriovEnabled: true
    sriovStatus:
      numVfs: 8
      totalVfs: 64
      vfs:
      - id: 0
        mac: "00:00:00:00:00:00"
        pciAddr: 0000:02:0a.0
        qos: 0
        spoofchk: true
        trust: false
        vfDriver: i40evf
        vlan: 0
      - id: 1
        mac: "00:00:00:00:00:00"
        pciAddr: 0000:02:0a.1
        qos: 0
        spoofchk: true
        trust: false
        vfDriver: i40evf
        vlan: 0
      - id: 2
        mac: "00:00:00:00:00:00"
        pciAddr: 0000:02:0a.2
        qos: 0
        spoofchk: true
        trust: false
        vfDriver: i40evf
        vlan: 0
      - id: 3
        mac: "00:00:00:00:00:00"
        pciAddr: 0000:02:0a.3
        qos: 0
        spoofchk: true
        trust: false
        vfDriver: i40evf
        vlan: 0
      - id: 4
        mac: "00:00:00:00:00:00"
        pciAddr: 0000:02:0a.4
        qos: 0
        spoofchk: true
        trust: false
        vfDriver: i40evf
        vlan: 0
      - id: 5
        mac: "00:00:00:00:00:00"
        pciAddr: 0000:02:0a.5
        qos: 0
        spoofchk: true
        trust: false
        vfDriver: i40evf
        vlan: 0
      - id: 6
        mac: "00:00:00:00:00:00"
        pciAddr: 0000:02:0a.6
        qos: 0
        spoofchk: true
        trust: false
        vfDriver: i40evf
        vlan: 0
      - id: 7
        mac: "00:00:00:00:00:00"
        pciAddr: 0000:02:0a.7
        qos: 0
        spoofchk: true
        trust: false
        vfDriver: i40evf
        vlan: 0
    vendorId: "8086"
  routes:
    ipv4:
    - dev: bond0
      gw: 145.40.65.94
    - dev: bond0
      dst: 10.0.0.0/8
      gw: 10.66.12.130
    - dev: bond0
      dst: 10.66.12.130/31
      src: 10.66.12.131
    - dev: bond0
      dst: 145.40.65.94/31
      src: 145.40.65.95
    - dev: tunl0
      dst: 10.20.51.192/26
      gw: 145.40.88.37
    - dev: tunl0
      dst: 10.20.100.0/26
      gw: 139.178.64.179
    - dev: tunl0
      dst: 10.20.143.192/26
      gw: 147.75.38.91
    - dev: tunl0
      dst: 10.20.172.128/26
      gw: 145.40.78.131
    - dev: cali37e8e21ed0f
      dst: 10.20.184.193/32
    - dev: cali98dc5434789
      dst: 10.20.184.194/32
    - dev: cali2af3952c1b8
      dst: 10.20.184.195/32
    - dev: calidd52bdd3866
      dst: 10.20.184.196/32
    - dev: calib2d6fb3ef2e
      dst: 10.20.184.197/32
    - dev: cali78a9b72fd4d
      dst: 10.20.184.198/32
    ipv6:
    - dev: lo
      dst: ::/96
    - dev: lo
      dst: 0.0.0.0/0
    - dev: lo
      dst: 2002:a00::/24
    - dev: lo
      dst: 2002:7f00::/24
    - dev: lo
      dst: 2002:a9fe::/32
    - dev: lo
      dst: 2002:ac10::/28
    - dev: lo
      dst: 2002:c0a8::/32
    - dev: lo
      dst: 2002:e000::/19
    - dev: lo
      dst: 3ffe:ffff::/32
    - dev: eno2
      dst: fe80::/64
    - dev: bond0
      dst: 2604:1380:45d1:2600::2/127
    - dev: bond0
      dst: fe80::/64
    - dev: bond0
      gw: 2604:1380:45d1:2600::2
    - dev: bond0
      gw: fe80::400:deff:fead:beef
    - dev: eno2.1000
      dst: fe80::/64
    - dev: eno2.1001
      dst: fe80::/64
    - dev: eno2.1002
      dst: fe80::/64
    - dev: cali37e8e21ed0f
      dst: fe80::/64
    - dev: cali98dc5434789
      dst: fe80::/64
    - dev: cali2af3952c1b8
      dst: fe80::/64
    - dev: calidd52bdd3866
      dst: fe80::/64
    - dev: calib2d6fb3ef2e
      dst: fe80::/64
    - dev: cali78a9b72fd4d
      dst: fe80::/64
```

We can see a detailed output of all SRIOV information, including each VF and it's PCI address. For example we can see eno1, which supports 64 VFs but does not have any configured yet. On eno2, we can see detailed info for each of the 8 VFs. We also see all other L2 link layer information for each device, along with the IPv4 and IPv6 routing tables
