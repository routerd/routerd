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
	"net"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
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
	// we poll against the cache,
	// so this number could be even lower
	leasePollInterval = 500 * time.Millisecond
	// how long we waat for the lease to be Bound until we give up.
	leaseAcquireTimeout = 6 * time.Second
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
		if errors.IsConflict(err) {
			continue
		}
		return nil
	}
}
