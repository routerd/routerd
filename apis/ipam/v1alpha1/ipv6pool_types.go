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

// IPv6PoolSpec defines the desired state of IPv6Pool
type IPv6PoolSpec struct {
	// Subnet CIDR that this Pool is managing.
	CIDR string `json:"cidr"`

	// Lease duration for leased ips.
	// IPv6Leases of type "Dynamic" must be renewed in time or
	// will be deleted and reclaimed into the pool.
	LeaseDuration *metav1.Duration `json:"leaseDuration,omitempty"`
}

// IPv6PoolStatus defines the observed state of IPv6Pool
type IPv6PoolStatus struct {
	// Number of all IPs available in the subnet.
	AvailableIPs int `json:"availableIPs"`
	// Number of allocated IPs in the subnet.
	AllocatedIPs int `json:"allocatedIPs"`
}

// IPv6Pool is the Schema for the ipv6pools API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="CIDR",type="string",JSONPath=".spec.cidr"
// +kubebuilder:printcolumn:name="Available",type="integer",JSONPath=".status.availableIPs"
// +kubebuilder:printcolumn:name="Allocated",type="integer",JSONPath=".status.allocatedIPs"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type IPv6Pool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPv6PoolSpec   `json:"spec,omitempty"`
	Status IPv6PoolStatus `json:"status,omitempty"`
}

// IPv6PoolList contains a list of IPPool
// +kubebuilder:object:root=true
type IPv6PoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPv6Pool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IPv6Pool{}, &IPv6PoolList{})
}
