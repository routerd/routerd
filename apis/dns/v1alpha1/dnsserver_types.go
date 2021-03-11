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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DNSServerSpec defines the desired state of DNSServer
type DNSServerSpec struct {
	// Configures forwarding queries to other DNS servers.
	Forward *DNSForward `json:"forward,omitempty"`
	// Enables caching of records.
	Cache *DNSCache `json:"cache,omitempty"`
	// Limits the delivered Zones and RecordSets.
	RecordSelector metav1.LabelSelector `json:"recordSelector,omitempty"`
	// NetworkAttachment configures how the DNS Server is attached to a network.
	NetworkAttachment NetworkAttachment `json:"networkAttachment"`
}

type NetworkAttachment struct {
	// References a NetworkAttachmentDefinition to attach the DNSServer to a network.
	NetworkAttachmentDefinition LocalObjectReference `json:"networkAttachmentDefinition"`
	// References an IPv4Pool to acquire the IP Adress for this DNS server from.
	IPv4Pool LocalObjectReference `json:"ipv4Pool,omitempty"`
	// References an IPv6Pool to acquire the IP Adress for this DNS server from.
	IPv6Pool LocalObjectReference `json:"ipv6Pool,omitempty"`
}

type DNSForward struct {
	// From is the base domain to match for the request to be forwarded.
	From string `json:"from"`
	// destination endpoints to forward to. The TO syntax allows you to specify a protocol,
	// tls://9.9.9.9 or dns:// (or no protocol) for plain DNS.
	// The number of upstreams is limited to 15.
	To []string `json:"to"`
	// 	policy specifies the policy to use for selecting upstream servers.
	// +kubebuilder:validation:Enum=Random;RoundRobin;Sequential
	// +kubebuilder:default="Random"
	Policy DNSForwardPolicy `json:"policy"`
	// List of domains to exclude from forwarding.
	// Requests that match none of these names will be passed through.
	Except []string `json:"except,omitempty"`
}

type DNSCache struct {
	// Each element in the cache is cached according to its TTL (with TTL as the max).
	// +kubebuilder:default="3600s"
	TTL metav1.Duration `json:"ttl"`
	// Zones it should cache for.
	// If empty, the zones from the configuration block are used.
	Zones []string `json:"zones,omitempty"`
}

type DNSForwardPolicy string

const (
	// random is a policy that implements random upstream selection.
	DNSForwardPolicyRandom DNSForwardPolicy = "Random"
	// round_robin is a policy that selects hosts based on round robin ordering.
	DNSForwardPolicyRoundRobin DNSForwardPolicy = "RoundRobin"
	// sequential is a policy that selects hosts based on sequential ordering.
	DNSForwardPolicySequential DNSForwardPolicy = "Sequential"
)

// DNSServerStatus defines the observed state of DNSServer
type DNSServerStatus struct {
	// The most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions is a list of status conditions ths object is in.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Human readable status aggregated from conditions.
	Phase string `json:"phase,omitempty"`
	// IPv4 Address the DHCP server is available at.
	IPv4Address string `json:"ipv4Address,omitempty"`
	// IPv6 Address the DHCP server is available at.
	IPv6Address string `json:"ipv6Address,omitempty"`
}

const (
	DNSServerAvailable = "Available"
)

// DNSServer is the Schema for the dnsservers API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type DNSServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DNSServerSpec   `json:"spec,omitempty"`
	Status DNSServerStatus `json:"status,omitempty"`
}

// DNSServerList contains a list of DNSServer
// +kubebuilder:object:root=true
type DNSServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DNSServer{}, &DNSServerList{})
}
