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

// DHCPServerSpec defines the desired state of DHCPServer.
type DHCPServerSpec struct {
	// DHCP IPv4 specific settings.
	IPv4 *DHCPServerIPv4 `json:"ipv4,omitempty"`
	// DHCP IPv6 specific settings.
	IPv6 *DHCPServerIPv6 `json:"ipv6,omitempty"`
	// References a network attachment definition to attach the DHCP Server Pod to a network.
	NetworkAttachmentDefinition LocalObjectReference `json:"networkAttachmentDefinition"`
}

// DHCP Server Settings for IPv4.
type DHCPServerIPv4 struct {
	// References the IPv4Pool for IPAM.
	// The DHCPServer will itself acquire an IPv4Lease from this IPv4Pool.
	Pool LocalObjectReference `json:"pool"`
	// Gateway IPv4 address of the router for this subnet.
	Gateway string `json:"gateway"`
	// NameServers to point clients to.
	NameServers []string `json:"nameServers,omitempty"`
}

// DHCP Server Settings for IPv6.
type DHCPServerIPv6 struct {
	// References the IPv6Pool for IPAM.
	// The DHCPServer will itself acquire an IPv6Lease from this IPv6Pool.
	Pool LocalObjectReference `json:"pool"`
	// Gateway IPv4 address of the router for this subnet.
	Gateway string `json:"gateway"`
	// NameServers to point clients to.
	NameServers []string `json:"nameServers,omitempty"`
}

const (
	// Available condition indicates whether the DHCP Server is alive and well.
	Available = "Available"
)

// DHCPServerStatus defines the observed state of DHCPServer
type DHCPServerStatus struct {
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

type LocalObjectReference struct {
	Name string `json:"name"`
}

// DHCPServer is the Schema for the dhcpservers API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="IPv4 Pool",type="string",JSONPath=".status.ipv4.ippool.name"
// +kubebuilder:printcolumn:name="IPv6 Pool",type="string",JSONPath=".status.ipv6.ippool.name"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type DHCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DHCPServerSpec   `json:"spec,omitempty"`
	Status DHCPServerStatus `json:"status,omitempty"`
}

// DHCPServerList contains a list of DHCPServer
// +kubebuilder:object:root=true
type DHCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DHCPServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DHCPServer{}, &DHCPServerList{})
}
