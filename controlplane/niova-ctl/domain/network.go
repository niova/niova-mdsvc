package domain

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// IsValidIP checks if the given string is a valid IP address
func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// FormatIPAddresses formats multiple IP addresses for summary display
func FormatIPAddresses(hv ctlplfl.Hypervisor) string {
	if len(hv.IPAddrs) <= 0 {
		return "no network info"
	}
	if len(hv.IPAddrs) == 1 {
		return hv.IPAddrs[0]
	}
	return fmt.Sprintf("%s +%d more", hv.IPAddrs[0], len(hv.IPAddrs)-1)
}

// FormatDetailedIPAddresses formats all IP addresses for detailed views
func FormatDetailedIPAddresses(hv ctlplfl.Hypervisor) string {
	if len(hv.IPAddrs) == 0 {
		return ""
	}
	if len(hv.IPAddrs) == 1 {
		return hv.IPAddrs[0]
	}
	result := fmt.Sprintf("IP Addresses: %s", hv.IPAddrs[0])
	for i := 1; i < len(hv.IPAddrs); i++ {
		result += fmt.Sprintf(", %s", hv.IPAddrs[i])
	}
	return result
}

// ParsePortRange parses a port range string like "8000-8100" into start and end ports
func ParsePortRange(portRange string) (int, int, error) {
	if portRange == "" {
		return 0, 0, fmt.Errorf("empty port range")
	}

	parts := strings.Split(portRange, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid port range format: %s", portRange)
	}

	start := 0
	end := 0
	var err error

	if start, err = strconv.Atoi(strings.TrimSpace(parts[0])); err != nil {
		return 0, 0, fmt.Errorf("invalid start port: %s", parts[0])
	}

	if end, err = strconv.Atoi(strings.TrimSpace(parts[1])); err != nil {
		return 0, 0, fmt.Errorf("invalid end port: %s", parts[1])
	}

	if start >= end {
		return 0, 0, fmt.Errorf("start port must be less than end port")
	}

	return start, end, nil
}

// AllocatePortPair allocates a contiguous pair of ports (serverPort=N, clientPort=N+1)
func AllocatePortPair(hypervisorUUID string, portRange string, userToken string, cpClient ControlPlaneClient) (int, int, error) {
	startPort, endPort, err := ParsePortRange(portRange)
	if err != nil {
		return 0, 0, err
	}

	allocatedPorts := make(map[int]bool)

	if cpClient != nil {
		cpClient.SetToken(userToken)
		nisds, err := cpClient.GetNisds()
		if err == nil {
			for _, nisd := range nisds {
				if nisd.FailureDomain[ctlplfl.HV_IDX] == hypervisorUUID {
					allocatedPorts[int(nisd.PeerPort)] = true
					allocatedPorts[int(nisd.NetInfo[0].Port)] = true
				}
			}
		}
	}

	for port := startPort; port < endPort-1; port += 2 {
		serverPort := port
		clientPort := port + 1

		if !allocatedPorts[serverPort] && !allocatedPorts[clientPort] {
			return clientPort, serverPort, nil
		}
	}

	return 0, 0, fmt.Errorf("no available port pairs in range %s", portRange)
}
