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

package radvd

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
)

var (
	ipNet      *net.IPNet
	fullPrefix Prefix
	fullRoute  Route
	fullRDNSS  = RDNSS{
		IPs: []net.IP{
			net.ParseIP("2001:db8::1"),
			net.ParseIP("2001:db8::2"),
			net.ParseIP("2001:db8::3"),
		},
		AdvRDNSSLifetime: (*time.Duration)(pointer.Int64Ptr(0)),
	}
	fullClients = Clients{
		net.ParseIP("2001:db8::1"),
		net.ParseIP("2001:db8::2"),
		net.ParseIP("2001:db8::3"),
	}
)

func init() {
	_, ipNet, _ = net.ParseCIDR("2001:db8::/32")
	fullPrefix = Prefix{
		Prefix:               *ipNet,
		AdvOnLink:            pointer.BoolPtr(true),
		AdvAutonomous:        pointer.BoolPtr(false),
		AdvRouterAddr:        pointer.BoolPtr(true),
		AdvValidLifetime:     48 * time.Hour,
		AdvPreferredLifetime: 24 * time.Hour,
		Base6to4Interface:    "eth0",
	}
	fullRoute = Route{
		Prefix:             *ipNet,
		AdvRouteLifetime:   4 * time.Hour,
		AdvRoutePreference: AdvPreferenceHigh,
	}
}

func TestConfig_String(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected string
	}{
		{
			name: "single interface",
			config: Config{
				Interfaces: []Interface{
					{Name: "eth0"},
				},
			},
			expected: `interface eth0
{
};
`,
		},
		{
			name: "multiple interfaces",
			config: Config{
				Interfaces: []Interface{
					{Name: "eth0"},
					{Name: "eth1"},
				},
			},
			expected: `interface eth0
{
};

interface eth1
{
};
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.config.String())
		})
	}
}

func TestPrefix_String(t *testing.T) {
	tests := []struct {
		name     string
		prefix   Prefix
		expected string
	}{
		{
			name: "unspecified options are omitted",
			prefix: Prefix{
				Prefix:    *ipNet,
				AdvOnLink: pointer.BoolPtr(true),
			},
			expected: `	prefix 2001:db8::/32
	{
		AdvOnLink on;
	};
`,
		},
		{
			name:   "complete",
			prefix: fullPrefix,
			expected: `	prefix 2001:db8::/32
	{
		AdvOnLink on;
		AdvAutonomous off;
		AdvRouterAddr on;
		AdvValidLifetime 172800;
		AdvPreferredLifetime 86400;
		Base6to4Interface eth0;
	};
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.prefix.String())
		})
	}
}

func TestRDNSS_String(t *testing.T) {
	tests := []struct {
		name     string
		rdnss    RDNSS
		expected string
	}{
		{
			name: "unspecified options are omitted",
			rdnss: RDNSS{
				IPs: []net.IP{
					net.ParseIP("2001:db8::1"),
					net.ParseIP("2001:db8::2"),
					net.ParseIP("2001:db8::3"),
				},
			},
			expected: `	RDNSS 2001:db8::1 2001:db8::2 2001:db8::3
	{
	};
`,
		},
		{
			name:  "complete",
			rdnss: fullRDNSS,
			expected: `	RDNSS 2001:db8::1 2001:db8::2 2001:db8::3
	{
		AdvRDNSSLifetime 0;
	};
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.rdnss.String())
		})
	}
}

func TestRoute_String(t *testing.T) {
	_, ipNet, _ := net.ParseCIDR("2001:db8::/32")

	tests := []struct {
		name     string
		route    Route
		expected string
	}{
		{
			name: "unspecified options are omitted",
			route: Route{
				Prefix: *ipNet,
			},
			expected: `	route 2001:db8::/32
	{
	};
`,
		},
		{
			name:  "complete",
			route: fullRoute,
			expected: `	route 2001:db8::/32
	{
		AdvRouteLifetime 14400;
		AdvRoutePreference high;
	};
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.route.String())
		})
	}
}

func TestClients_String(t *testing.T) {
	clients := fullClients
	expected := `	clients
	{
		2001:db8::1;
		2001:db8::2;
		2001:db8::3;
	};
`

	assert.Equal(t, expected, clients.String())
}

func TestInterface_String(t *testing.T) {
	iface := Interface{
		Name:                 "eth0",
		IgnoreIfMissing:      pointer.BoolPtr(true),
		AdvSendAdvert:        pointer.BoolPtr(true),
		UnicastOnly:          pointer.BoolPtr(true),
		MaxRtrAdvInterval:    20 * time.Millisecond,
		MinRtrAdvInterval:    10 * time.Millisecond,
		MinDelayBetweenRAs:   2 * time.Second,
		AdvManagedFlag:       pointer.BoolPtr(false),
		AdvOtherConfigFlag:   pointer.BoolPtr(true),
		AdvLinkMTU:           1280,
		AdvReachableTime:     520 * time.Millisecond,
		AdvRetransTimer:      420 * time.Millisecond,
		AdvCurHopLimit:       intPtr(62),
		AdvDefaultLifetime:   (*time.Duration)(pointer.Int64Ptr(0)),
		AdvDefaultPreference: AdvPreferenceHigh,
		AdvSourceLLAddress:   pointer.BoolPtr(false),
		AdvHomeAgentFlag:     pointer.BoolPtr(true),
		AdvHomeAgentInfo:     pointer.BoolPtr(true),
		HomeAgentLifetime:    (*time.Duration)(pointer.Int64Ptr(int64(30 * time.Second))),
		HomeAgentPreference:  intPtr(-4),
		AdvMobRtrSupportFlag: pointer.BoolPtr(true),
		AdvIntervalOpt:       pointer.BoolPtr(true),

		Prefix:  []Prefix{fullPrefix},
		Routes:  []Route{fullRoute},
		RDNSS:   []RDNSS{fullRDNSS},
		Clients: fullClients,
	}
	expected := `interface eth0
{
	IgnoreIfMissing on;
	AdvSendAdvert on;
	UnicastOnly on;
	MaxRtrAdvInterval 0.02;
	MinRtrAdvInterval 0.01;
	MinDelayBetweenRAs 2;
	AdvManagedFlag off;
	AdvOtherConfigFlag on;
	AdvLinkMTU 1280;
	AdvReachableTime 520;
	AdvRetransTimer 420;
	AdvCurHopLimit 62;
	AdvDefaultLifetime 0;
	AdvDefaultPreference high;
	AdvSourceLLAddress off;
	AdvHomeAgentFlag on;
	AdvHomeAgentInfo on;
	HomeAgentLifetime 3;
	HomeAgentPreference -4;
	AdvMobRtrSupportFlag on;
	AdvIntervalOpt on;

	prefix 2001:db8::/32
	{
		AdvOnLink on;
		AdvAutonomous off;
		AdvRouterAddr on;
		AdvValidLifetime 172800;
		AdvPreferredLifetime 86400;
		Base6to4Interface eth0;
	};

	route 2001:db8::/32
	{
		AdvRouteLifetime 14400;
		AdvRoutePreference high;
	};

	RDNSS 2001:db8::1 2001:db8::2 2001:db8::3
	{
		AdvRDNSSLifetime 0;
	};

	clients
	{
		2001:db8::1;
		2001:db8::2;
		2001:db8::3;
	};
};
`

	assert.Equal(t, expected, iface.String())
}

func intPtr(i int) *int {
	return &i
}
