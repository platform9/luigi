package constants

const (
	CalicoPlugin         = "calico"
	OvsPlugin            = "ovs"
	VMFinalizerName      = "teardownVM"
	NetworkFinalizerName = "teardownNetwork"

	Pf9NetworkAnnotation = "plumber.k8s.pf9.io/networkName"
	Pf9VMIServiceLabel   = "plumber.k8s.pf9.io/vmService"
)
