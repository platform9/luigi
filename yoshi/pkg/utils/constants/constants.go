package constants

const (
	CalicoPlugin         = "calico"
	OvsPlugin            = "ovs"
	VMFinalizerName      = "vm.plumber.k8s.pf9.io"
	NetworkFinalizerName = "network.plumber.k8s.pf9.io"

	Pf9NetworkAnnotation  = "plumber.k8s.pf9.io/networkName"
	Pf9VMIServiceLabel    = "plumber.k8s.pf9.io/vmService"
	Pf9PublicIPAnnotation = "plumber.k8s.pf9.io/publicIP"
)
