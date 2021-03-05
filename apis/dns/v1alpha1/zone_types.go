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

// SOARecord represents the SOA record for this zone.
type SOARecord struct {
	TTL         metav1.Duration `json:"ttl"`
	Master      string          `json:"master"`
	Admin       string          `json:"admin"`
	Serial      int             `json:"serial"`
	Refresh     metav1.Duration `json:"refresh"`
	Retry       metav1.Duration `json:"retry"`
	Expire      metav1.Duration `json:"expire"`
	NegativeTTL metav1.Duration `json:"negativeTTL"`
}

type ZoneConfig struct {
	// start of authority record
	SOA SOARecord `json:"soa"`
}

// Zone is the Schema for the zones API
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Master",type="string",JSONPath=".zone.soa.master"
// +kubebuilder:printcolumn:name="Admin",type="string",JSONPath=".zone.soa.admin"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type Zone struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	ZoneConfig `json:"zone"`
}

// ZoneList contains a list of Zone
// +kubebuilder:object:root=true
type ZoneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Zone `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Zone{}, &ZoneList{})
}
