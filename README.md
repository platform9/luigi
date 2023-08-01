# Luigi
Luigi is a Kubernetes Operator to deploy, manage, and upgrade advanced networking plugins. The default Kubernetes networking model with one CNI and cluster-wide network can be too restrictive for many advanced networking use cases like NFV or virtualization

There are many discrete plugins and solutions, but knowing which ones to use, deploying and managing them can be tedious. Secondary CNIs? Multus? SRIOV? Device plugins? OVS? Which IPAM? What's the current Linux networking state of my nodes? How do I configure my nodes in order to support all of these?

# How to deploy
This will require an already working K8s cluster with DNS and a primary CNI up and running. 
Deploy the manifest found in samples in this repo:
```
kubectl apply -f https://raw.githubusercontent.com/platform9/luigi/master/samples/luigi-plugins-operator.yaml
```
A deployment of 1 replica will be created in the luigi-system namespace.

Or, To get started sign up for Platform9 Managed Kubernetes(PMK) for free at platform9.com/signup, see more about our Telco 5G offerings at platform9.com/solutions/telco-5g or contact us at platform9.com/contact. With PMK, Luigi will already be deployed and managed itself

# Plugins supported
The scope of each plugin is beyond this documentation. But if you know you need it, luigi will deploy the following:

 - HostPlumber: A subset of Luigi, an operator to configure/prep networking on the node and retrieve node details
	 - See: https://github.com/platform9/luigi/blob/master/hostplumber/README.md
	 - Use to create SRIOV VFs, configure OVS, create VLAN interfaces, etc...
	 - Recommended unless you have your own tooling to configure nodes
 - Multus
	 - Almost always required - the only way K8S can support multiple CNIs and networks
 - SRIOV CNI
 - SRIOV Device Plugin
 - OpenVSwitch daemon & CLI tools
 - OVS CNI plugin
 - Macvlan, IPvlan
 - Whereabouts IPAM driver
	 - Required for dynamic IP assignment without an external DHCP service.
 - Node Feature Discovery

# Configuration:

**namespace**: Each plugin will take in a namespace override to deploy, default namespace otherwise

**Image override:** Each plugin will take an image override field to use a non-default/stable container image. This is not guaranteed to work, especially if the plugin's CRs have changed. It should only be used for dev-test or bug fixes

**imagePullPolicy:** By default IfNotPresent

**privateRegistryBase**: Some airgapped env's may have a custom container registry. If this is specified, it will replace the public container registry URL (docker.io, gcr.io, quay, etc..) with this path

Each plugin may or may not have some further specific configuration. Here are the current options as of release v0.3:
 - HostPlumber - none
 - Multus - none
 - SRIOV - none
 - Node-feature-discovery - none
 - OVS - none
 - Whereabouts
	 - ipReconcilerSchedule - specify the CronJob schedule of the whereabouts IP cleanup Job
	 - ipReconcilerNodeSelector - specify the nodeSelector Labels on which to schedule the ip-reconciler

# NetworkPlugins CRD:
In it's current phase, only one instance of the CRD is supported. It will reflect the final, desired state of all plugins to be deployed.

If it is present, Luigi will ensure that the plugin is deployed and upgraded. If missing and re-applied, Luigi will remove the plugin if it was previously managing it.

```
apiVersion: plumber.k8s.pf9.io/v1
kind: NetworkPlugins
metadata:
  name: networkplugins-sample11
spec:
  # Add fields here
  #privateRegistryBase: "localhost:5100"
  plugins:
    hostPlumber: {}
    nodeFeatureDiscovery: {}
    multus: {}
    whereabouts:
      ipReconcilerSchedule: "*/1 * * * *"
      ipReconcilerNodeSelector:
         foo: bar
    # SRIOV actually consists of two plugins - the CNI, and the device-plugin
    # Use HostPlumber to create the actual VFs
    sriov: {}
    ovs: {}
```

The above will deploy all the plugins specified in the default namespace. To override the namespace, and deploy in kube-system:

```
apiVersion: plumber.k8s.pf9.io/v1
kind: NetworkPlugins
metadata:
  name: networkplugins-sample11
spec:
  # Add fields here
  #privateRegistryBase: "localhost:5100"
  plugins:
    hostPlumber:
      namespace: "kube-system"
    nodeFeatureDiscovery: {}
    multus:
      namespace: "kube-system"
    whereabouts:
      namespace: "kube-system"
    sriov:
      namespace: "kube-system"
```

That is it! Now that you have the secondary CNIs and other related plugins deployed, you may need to prep the nodes before you can actually create Multus Networks and assign them to Pods. In order to do so, use Luigi's own HostPlumber plugin: https://github.com/platform9/luigi/blob/master/hostplumber/README.md


##### Dev note
This project needs to migrate to Kubebuilder/v4.
webhooks where added manually `make generate && make manifestes` will not add required feild for webhook in crds and luigi deployment. refer `samples/luigi-plugins-operator-v2.yaml`