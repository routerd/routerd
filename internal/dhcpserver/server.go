/*
Copyright 2021 The routerd authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dhcpserver

import (
	"errors"
	"fmt"
	"net"
	"time"

	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	dhcpMACLabel  = "dhcp.routerd.net/mac"
	dhcpHostLabel = "dhcp.routerd.net/host"
	dhcpDUIDLabel = "dhcp.routerd.net/duid"
	dhcpFQDNLabel = "dhcp.routerd.net/fqdn"
)

type Config struct {
	// Interface to bind the DHCP server onto.
	BindInterface string
	// Kubernetes Namespace we are running in.
	Namespace string
	// Name of the DHCPServer to query the configuration from.
	DHCPServerName string
}

const (
	// we poll against the cache, so this number can be very lower
	leasePollInterval = 100 * time.Millisecond
	// how long we wait for the lease to be Bound until we give up.
	leaseAcquireTimeout = 2 * time.Second
)

func stringSliceToIPSlice(ipstrings []string) []net.IP {
	out := make([]net.IP, len(ipstrings))
	for _, ipstring := range ipstrings {
		out = append(out, net.ParseIP(ipstring))
	}
	return out
}

func retryOnConflict(fn func() error) error {
	for {
		err := fn()
		if k8sApiErrors.IsConflict(err) {
			continue
		}
		return nil
	}
}

func getInterfaceIPv4IPs(ifaceName string) ([]net.IP, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("could not get interface: %w", err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("could not get interface addresses: %w", err)
	}

	var ips []net.IP
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			return nil, fmt.Errorf("could not parse IP from interface address %w", err)
		}
		ips = append(ips, ip)
	}

	return ips, nil
}

func getInterfaceIPv4InCIDR(iface string, cidr string) (net.IP, error) {
	ips, err := getInterfaceIPv4IPs(iface)
	if err != nil {
		return nil, err
	}

	_, netIPNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	fmt.Printf("netIPNet: %+v\n", netIPNet)

	for _, ip := range ips {
		if netIPNet.Contains(ip) {
			return ip, nil
		}
	}

	return nil, errors.New("no ipv4 on the interface matched the given CIDR")
}
