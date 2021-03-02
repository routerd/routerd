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
	// References the NetworkAttachmentDefinition to attach the DNSServer to a network.
	NetworkAttachmentDefinition LocalObjectReference `json:"networkAttachmentDefinition"`
}

// DNSServerStatus defines the observed state of DNSServer
type DNSServerStatus struct {
	// The most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions is a list of status conditions ths object is in.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Human readable status aggregated from conditions.
	Phase string `json:"phase,omitempty"`
}

// DNSServer is the Schema for the dnsservers API
// +kubebuilder:object:root=true
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
