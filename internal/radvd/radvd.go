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
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Interfaces []Interface
}

func (c Config) String() string {
	var s string
	for i, iface := range c.Interfaces {
		if i > 0 {
			s += "\n"
		}
		s += iface.String()
	}
	return s
}

type Interface struct {
	// Interface name.
	Name string
	// A flag indicating whether or not the interface is ignored if it does not exist at start-up. By default, radvd exits.
	// This is useful for dynamic interfaces which are not active when radvd starts or which are dynamically disabled and re-enabled during the time radvd runs.
	// Current versions of radvd automatically try to re-enable interfaces.
	// Enabling IgnoreIfMissing also quenches certain warnings in log messages relating to missing interfaces.
	// Default: off
	IgnoreIfMissing *bool
	// A flag indicating whether or not the router sends periodic router advertisements and responds to router solicitations.
	// Default: off
	AdvSendAdvert *bool
	// Indicates that the interface link type only supports unicast. This will prevent unsolicited advertisements from being sent, and will cause solicited advertisements to be unicast to the soliciting node. This option is necessary for non-broadcast, multiple-access links, such as ISATAP.
	// Default: off
	UnicastOnly *bool
	// The maximum time allowed between sending unsolicited multicast router advertisements from the interface, in seconds.
	// Must be no less than 4 seconds and no greater than 1800 seconds.
	// Minimum when using Mobile IPv6 extensions: 0.07.
	// For values less than 0.2 seconds, 0.02 seconds is added to account for scheduling granularities as specified in RFC3775.
	// Default: 600 seconds
	MaxRtrAdvInterval time.Duration
	// The minimum time allowed between sending unsolicited multicast router advertisements from the interface, in seconds.
	// Must be no less than 3 seconds and no greater than 0.75 * MaxRtrAdvInterval.
	// Minimum when using Mobile IPv6 extensions: 0.03.
	// Default: 0.33 * MaxRtrAdvInterval
	MinRtrAdvInterval time.Duration
	//     The minimum time allowed between sending multicast router advertisements from the interface, in seconds.
	// This applies to solicited multicast RAs. This is defined as the protocol constant MIN_DELAY_BETWEEN_RAS in RFC4861. MIPv6 redefines this parameter to have a minimum of 0.03 seconds.
	// Minimum when using Mobile IPv6 extensions: 0.03.
	// Default: 3s
	MinDelayBetweenRAs time.Duration
	// When set, hosts use the administered (stateful) protocol for address autoconfiguration in addition to any addresses autoconfigured using stateless address autoconfiguration. The use of this flag is described in RFC 4862.
	// Default: off
	AdvManagedFlag *bool
	// When set, hosts use the administered (stateful) protocol for autoconfiguration of other (non-address) information. The use of this flag is described in RFC 4862.
	// Default: off
	AdvOtherConfigFlag *bool
	// The MTU option is used in router advertisement messages to insure that all nodes on a link use the same MTU value in those cases where the link MTU is not well known.
	// If specified, i.e. not 0, must not be smaller than 1280 and not greater than the maximum MTU allowed for this link (e.g. ethernet has a maximum MTU of 1500. See RFC 4864).
	// Default: 0
	AdvLinkMTU int
	// The time, in milliseconds, that a node assumes a neighbor is reachable after having received a reachability confirmation. Used by the Neighbor Unreachability Detection algorithm (see Section 7.3 of RFC 4861). A value of zero means unspecified (by this router).
	// Must be no greater than 3,600,000 milliseconds (1 hour).
	// Default: 0
	AdvReachableTime time.Duration
	// The time, in milliseconds, between retransmitted Neighbor Solicitation messages. Used by address resolution and the Neighbor Unreachability Detection algorithm (see Sections 7.2 and 7.3 of RFC 4861). A value of zero means unspecified (by this router).
	// Default: 0
	AdvRetransTimer time.Duration
	// The default value that should be placed in the Hop Count field of the IP header for outgoing (unicast) IP packets. The value should be set to the current diameter of the Internet. The value zero means unspecified (by this router).
	// Default: 64
	AdvCurHopLimit *int
	// The lifetime associated with the default router in units of seconds. The maximum value corresponds to 18.2 hours. A lifetime of 0 indicates that the router is not a default router and should not appear on the default router list. The router lifetime applies only to the router's usefulness as a default router; it does not apply to information contained in other message fields or options. Options that need time limits for their information include their own lifetime fields.
	// Must be either zero or between MaxRtrAdvInterval and 9000 seconds.
	// Default: 3 * MaxRtrAdvInterval (Minimum 1 second).
	AdvDefaultLifetime *time.Duration
	// The preference associated with the default router, as either "low", "medium", or "high".
	// Default: medium
	AdvDefaultPreference AdvPreferenceType
	// When set, the link-layer address of the outgoing interface is included in the RA.
	// Default: on
	AdvSourceLLAddress *bool
	// When set, indicates that sending router is able to serve as Mobile IPv6 Home Agent. When set, minimum limits specified by Mobile IPv6 are used for MinRtrAdvInterval and MaxRtrAdvInterval.
	// Default: off
	AdvHomeAgentFlag *bool
	// When set, Home Agent Information Option (specified by Mobile IPv6) is included in Router Advertisements. AdvHomeAgentFlag must also be set when using this option.
	// Default: off
	AdvHomeAgentInfo *bool
	// The length of time in seconds (relative to the time the packet is sent) that the router is offering Mobile IPv6 Home Agent services. A value 0 must not be used. The maximum lifetime is 65520 seconds (18.2 hours). This option is ignored, if AdvHomeAgentInfo is not set.
	// If both HomeAgentLifetime and HomeAgentPreference are set to their default values, Home Agent Information Option will not be sent.
	// Default: AdvDefaultLifetime
	HomeAgentLifetime *time.Duration
	// The preference for the Home Agent sending this Router Advertisement. Values greater than 0 indicate more preferable Home Agent, values less than 0 indicate less preferable Home Agent. This option is ignored, if AdvHomeAgentInfo is not set.
	// If both HomeAgentLifetime and HomeAgentPreference are set to their default values, Home Agent Information Option will not be sent.
	// Default: 0
	HomeAgentPreference *int
	// When set, the Home Agent signals it supports Mobile Router registrations (specified by NEMO Basic). AdvHomeAgentInfo must also be set when using this option.
	// Default: off
	AdvMobRtrSupportFlag *bool
	// When set, Advertisement Interval Option (specified by Mobile IPv6) is included in Router Advertisements. When set, minimum limits specified by Mobile IPv6 are used for MinRtrAdvInterval and MaxRtrAdvInterval.
	// The advertisement interval is based on the configured MaxRtrAdvInterval parameter except where this is less than 200ms. In this case, the advertised interval is ( MaxRtrAdvInterval + 20ms ).
	// Default: off
	AdvIntervalOpt *bool

	Prefix  []Prefix
	Routes  []Route
	RDNSS   []RDNSS
	Clients Clients
}

func (i Interface) String() string {
	s := "interface " + i.Name + "\n{\n"
	if i.IgnoreIfMissing != nil {
		s += "\tIgnoreIfMissing " + onOrOff(*i.IgnoreIfMissing) + ";\n"
	}
	if i.AdvSendAdvert != nil {
		s += "\tAdvSendAdvert " + onOrOff(*i.AdvSendAdvert) + ";\n"
	}
	if i.UnicastOnly != nil {
		s += "\tUnicastOnly " + onOrOff(*i.UnicastOnly) + ";\n"
	}
	if i.MaxRtrAdvInterval != 0 {
		s += "\tMaxRtrAdvInterval " + subSecond(i.MaxRtrAdvInterval) + ";\n"
	}
	if i.MinRtrAdvInterval != 0 {
		s += "\tMinRtrAdvInterval " + subSecond(i.MinRtrAdvInterval) + ";\n"
	}
	if i.MinDelayBetweenRAs != 0 {
		s += "\tMinDelayBetweenRAs " + subSecond(i.MinDelayBetweenRAs) + ";\n"
	}
	if i.AdvManagedFlag != nil {
		s += "\tAdvManagedFlag " + onOrOff(*i.AdvManagedFlag) + ";\n"
	}
	if i.AdvOtherConfigFlag != nil {
		s += "\tAdvOtherConfigFlag " + onOrOff(*i.AdvOtherConfigFlag) + ";\n"
	}
	if i.AdvLinkMTU != 0 {
		s += "\tAdvLinkMTU " + strconv.Itoa(i.AdvLinkMTU) + ";\n"
	}
	if i.AdvReachableTime != 0 {
		s += "\tAdvReachableTime " + strconv.Itoa(int(i.AdvReachableTime.Milliseconds())) + ";\n"
	}
	if i.AdvRetransTimer != 0 {
		s += "\tAdvRetransTimer " + strconv.Itoa(int(i.AdvRetransTimer.Milliseconds())) + ";\n"
	}
	if i.AdvCurHopLimit != nil {
		s += "\tAdvCurHopLimit " + strconv.Itoa(*i.AdvCurHopLimit) + ";\n"
	}
	if i.AdvDefaultLifetime != nil {
		s += "\tAdvDefaultLifetime " + subSecond(*i.AdvDefaultLifetime) + ";\n"
	}
	if len(i.AdvDefaultPreference) > 0 {
		s += "\tAdvDefaultPreference " + string(i.AdvDefaultPreference) + ";\n"
	}
	if i.AdvSourceLLAddress != nil {
		s += "\tAdvSourceLLAddress " + onOrOff(*i.AdvSourceLLAddress) + ";\n"
	}
	if i.AdvHomeAgentFlag != nil {
		s += "\tAdvHomeAgentFlag " + onOrOff(*i.AdvHomeAgentFlag) + ";\n"
	}
	if i.AdvHomeAgentInfo != nil {
		s += "\tAdvHomeAgentInfo " + onOrOff(*i.AdvHomeAgentInfo) + ";\n"
	}
	if i.HomeAgentLifetime != nil {
		s += "\tHomeAgentLifetime " + subSecond(*i.HomeAgentLifetime) + ";\n"
	}
	if i.HomeAgentPreference != nil {
		s += "\tHomeAgentPreference " + strconv.Itoa(*i.HomeAgentPreference) + ";\n"
	}
	if i.AdvMobRtrSupportFlag != nil {
		s += "\tAdvMobRtrSupportFlag " + onOrOff(*i.AdvMobRtrSupportFlag) + ";\n"
	}
	if i.AdvIntervalOpt != nil {
		s += "\tAdvIntervalOpt " + onOrOff(*i.AdvIntervalOpt) + ";\n"
	}
	for _, p := range i.Prefix {
		s += "\n" + p.String()
	}
	for _, r := range i.Routes {
		s += "\n" + r.String()
	}
	for _, rdnss := range i.RDNSS {
		s += "\n" + rdnss.String()
	}
	if len(i.Clients) > 0 {
		s += "\n" + i.Clients.String()
	}
	return s + "};\n"
}

type Prefix struct {
	// Prefix these settings should belong to.
	Prefix net.IPNet
	// When set, indicates that this prefix can be used for on-link determination. When not set the advertisement makes no statement about on-link or off-link properties of the prefix. For instance, the prefix might be used for address configuration with some of the addresses belonging to the prefix being on-link and others being off-link.
	// Default: on
	AdvOnLink *bool
	// When set, indicates that this prefix can be used for autonomous address configuration as specified in RFC 4862.
	// Default: on
	AdvAutonomous *bool
	// When set, indicates that the address of interface is sent instead of network prefix, as is required by Mobile IPv6. When set, minimum limits specified by Mobile IPv6 are used for MinRtrAdvInterval and MaxRtrAdvInterval.
	// Default: off
	AdvRouterAddr *bool
	// The length of time in seconds (relative to the time the packet is sent) that the prefix is valid for the purpose of on-link determination. The symbolic value infinity represents infinity (i.e. a value of all one bits (0xffffffff)). The valid lifetime is also used by RFC 4862.
	// Note that clients will ignore AdvValidLifetime of an existing prefix if the lifetime is below two hours, as required in RFC 4862 Section 5.5.3 point e).
	// Note: RFC4861's suggested default value is significantly longer: 30 days.
	// Default: 86400 seconds (1 day)
	AdvValidLifetime time.Duration
	// The length of time in seconds (relative to the time the packet is sent) that addresses generated from the prefix via stateless address autoconfiguration remain preferred. The symbolic value infinity represents infinity (i.e. a value of all one bits (0xffffffff)). See RFC 4862.
	// Note: RFC4861's suggested default value is significantly longer: 7 days.
	// Default: 14400 seconds (4 hours)
	AdvPreferredLifetime time.Duration
	// If this option is specified, this prefix will be combined with the IPv4 address of interface name to produce a valid 6to4 prefix. The first 16 bits of this prefix will be replaced by 2002 and the next 32 bits of this prefix will be replaced by the IPv4 address assigned to interface name at configuration time. The remaining 80 bits of the prefix (including the SLA ID) will be advertised as specified in the configuration file. See the next section for an example.
	// If interface name is not available at configuration time, a warning will be written to the log and this prefix will be disabled until radvd is reconfigured.
	// This option enables systems with dynamic IPv4 addresses to update their advertised 6to4 prefixes simply by restarting radvd or sending a SIGHUP signal to cause radvd to reconfigure itself.
	// Note that 6to4 prefixes derived from dynamically-assigned IPv4 addresses should be advertised with a significantly shorter lifetime (see the AdvValidLifetime and AdvPreferredLifetime options).
	// For more information on 6to4, see RFC 3056.
	// Default: 6to4 is not used
	Base6to4Interface string
}

func (p Prefix) String() string {
	s := "\tprefix " + p.Prefix.String() + "\n\t{\n"
	if p.AdvOnLink != nil {
		s += "\t\tAdvOnLink " + onOrOff(*p.AdvOnLink) + ";\n"
	}
	if p.AdvAutonomous != nil {
		s += "\t\tAdvAutonomous " + onOrOff(*p.AdvAutonomous) + ";\n"
	}
	if p.AdvRouterAddr != nil {
		s += "\t\tAdvRouterAddr " + onOrOff(*p.AdvRouterAddr) + ";\n"
	}
	if p.AdvValidLifetime != 0 {
		s += "\t\tAdvValidLifetime " + strconv.Itoa(int(p.AdvValidLifetime.Seconds())) + ";\n"
	}
	if p.AdvPreferredLifetime != 0 {
		s += "\t\tAdvPreferredLifetime " + strconv.Itoa(int(p.AdvPreferredLifetime.Seconds())) + ";\n"
	}
	if len(p.Base6to4Interface) > 0 {
		s += "\t\tBase6to4Interface " + p.Base6to4Interface + ";\n"
	}
	return s + "\t};\n"
}

type Route struct {
	Prefix net.IPNet
	// The lifetime associated with the route in units of seconds. The symbolic value infinity represents infinity (i.e. a value of all one bits (0xffffffff)).
	// Default: 3 * MaxRtrAdvInterval
	AdvRouteLifetime time.Duration
	// The preference associated with the default router, as either "low", "medium", or "high".
	// Default: medium
	AdvRoutePreference AdvPreferenceType
}

func (r Route) String() string {
	s := "\troute " + r.Prefix.String() + "\n\t{\n"
	if r.AdvRouteLifetime != 0 {
		s += "\t\tAdvRouteLifetime " +
			strconv.Itoa(int(r.AdvRouteLifetime.Seconds())) + ";\n"
	}
	if len(r.AdvRoutePreference) > 0 {
		s += "\t\tAdvRoutePreference " +
			string(r.AdvRoutePreference) + ";\n"
	}
	return s + "\t};\n"
}

type RDNSS struct {
	IPs []net.IP
	// The maximum duration how long the RDNSS entries are used for name resolution. A value of 0 means the nameserver should no longer be used. The maximum duration how long the RDNSS entries are used for name resolution. A value of 0 means the nameserver should no longer be used. The value, if not 0, must be at least MaxRtrAdvInterval. To ensure stale RDNSS info gets removed in a timely fashion, this should not be greater than 2*MaxRtrAdvInterval.
	// Default: 2*MaxRtrAdvInterval
	AdvRDNSSLifetime *time.Duration
}

func (r RDNSS) String() string {
	var ips []string
	for _, ip := range r.IPs {
		ips = append(ips, ip.String())
	}

	s := "\tRDNSS " + strings.Join(ips, " ") + "\n\t{\n"
	if r.AdvRDNSSLifetime != nil {
		s += "\t\tAdvRDNSSLifetime " +
			strconv.Itoa(int(r.AdvRDNSSLifetime.Seconds())) + ";\n"
	}
	return s + "\t};\n"
}

type Clients []net.IP

func (c Clients) String() string {
	s := "\tclients\n\t{\n"
	for _, ip := range c {
		s += "\t\t" + ip.String() + ";\n"
	}
	return s + "\t};\n"
}

type AdvPreferenceType string

const (
	AdvPreferenceLow    AdvPreferenceType = "low"
	AdvPreferenceMedium AdvPreferenceType = "medium"
	AdvPreferenceHigh   AdvPreferenceType = "high"
)

func onOrOff(setting bool) string {
	if setting {
		return "on"
	}
	return "off"
}

func subSecond(d time.Duration) string {
	v := strings.TrimRight(fmt.Sprintf("%f", d.Seconds()), "0.")
	if len(v) > 0 {
		return v
	}
	return "0"
}
