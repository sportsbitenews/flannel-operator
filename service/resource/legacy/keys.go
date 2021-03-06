package legacy

import (
	"fmt"
	"strings"

	"github.com/giantswarm/flanneltpr"
	"strconv"
)

const (
	// networkApp is the app label for resources running flannel
	// components.
	networkApp = "flannel-network"
	// networkApp is the app label for resources cleaning flannel network
	// and bridges.
	destroyerApp = "flannel-destroyer"
	// base port for liveness probes
	portBase = 21000
	// health endpoint
	healthEndpoint = "/healthz"
	// liveness probe host
	probeHost = "127.0.0.1"
	// liveness config
	initialDelaySeconds = 10
	timeoutSeconds      = 5
	periodSeconds       = 10
	failureThreshold    = 2
	successThreshold    = 1
)

// networkNamespace returns the namespace in which the operator's resources run
// in.
func networkNamespace(spec flanneltpr.Spec) string {
	return networkApp + "-" + clusterID(spec)
}

// destroyerNamespace returns the namespace in which resources performing
// cleanup run in.
func destroyerNamespace(spec flanneltpr.Spec) string {
	return destroyerApp + "-" + clusterID(spec)
}

func clusterCustomer(spec flanneltpr.Spec) string {
	return spec.Cluster.Customer
}

func clusterID(spec flanneltpr.Spec) string {
	return spec.Cluster.ID
}

func clusterName(spec flanneltpr.Spec) string {
	return clusterID(spec)
}

func clusterNamespace(spec flanneltpr.Spec) string {
	return spec.Cluster.Namespace
}

func etcdNetworkConfigPath(spec flanneltpr.Spec) string {
	return etcdNetworkPath(spec) + "/config"
}

func etcdNetworkPath(spec flanneltpr.Spec) string {
	return "coreos.com/network/" + networkBridgeName(spec)
}

func flannelDockerImage(spec flanneltpr.Spec) string {
	return spec.Flannel.Docker.Image
}

func flannelRunDir(spec flanneltpr.Spec) string {
	return spec.Flannel.Spec.RunDir
}

func healthListenAddress(spec flanneltpr.Spec) string {
	return "http://" + probeHost + ":" + strconv.Itoa(int(livenessPort(spec)))
}
func hostPrivateNetwork(spec flanneltpr.Spec) string {
	return spec.Bridge.Spec.PrivateNetwork
}

func livenessPort(spec flanneltpr.Spec) int32 {
	return int32(portBase + spec.Flannel.Spec.VNI)
}

func networkBridgeDockerImage(spec flanneltpr.Spec) string {
	return spec.Bridge.Docker.Image
}
func networkHealthDockerImage(spec flanneltpr.Spec) string {
	return spec.Health.Docker.Image
}

func networkBridgeName(spec flanneltpr.Spec) string {
	return "br-" + clusterID(spec)
}

func networkDNSBlock(spec flanneltpr.Spec) string {
	var parts []string
	for _, s := range spec.Bridge.Spec.DNS.Servers {
		parts = append(parts, fmt.Sprintf("DNS=%s", s.String()))
	}
	return strings.Join(parts, "\n")
}

func networkEnvFilePath(spec flanneltpr.Spec) string {
	return fmt.Sprintf("%s/networks/%s.env", flannelRunDir(spec), networkBridgeName(spec))
}

func networkFlannelDevice(spec flanneltpr.Spec) string {
	return fmt.Sprintf("flannel.%d", spec.Flannel.Spec.VNI)
}

func networkInterfaceName(spec flanneltpr.Spec) string {
	return spec.Bridge.Spec.Interface
}

func networkNTPBlock(spec flanneltpr.Spec) string {
	var parts []string
	for _, s := range spec.Bridge.Spec.NTP.Servers {
		parts = append(parts, fmt.Sprintf("NTP=%s", s))
	}
	return strings.Join(parts, "\n")
}

func networkTapName(spec flanneltpr.Spec) string {
	return "tap-" + clusterID(spec)
}
