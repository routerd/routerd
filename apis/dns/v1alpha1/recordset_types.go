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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetType returns the type of the record.
func (r *RecordSet) GetType() RecordType {
	switch {
	case len(r.A) > 0:
		return RecordTypeA
	case len(r.AAAA) > 0:
		return RecordTypeAAAA
	case len(r.TXT) > 0:
		return RecordTypeTXT
	case r.CName != nil:
		return RecordTypeCName
	case len(r.NS) > 0:
		return RecordTypeNS
	case len(r.MX) > 0:
		return RecordTypeMX
	case len(r.SRV) > 0:
		return RecordTypeSRV
	default:
		return RecordTypeUnknown
	}
}

func (r *RecordSet) Values() []string {
	var values []string
	switch r.GetType() {
	case RecordTypeA:
		values = r.A
	case RecordTypeAAAA:
		values = r.AAAA
	case RecordTypeTXT:
		values = r.TXT
	case RecordTypeCName:
		values = []string{*r.CName}
	case RecordTypeNS:
		values = r.NS
	case RecordTypeMX:
		for _, mx := range r.MX {
			values = append(values, mx.String())
		}
	case RecordTypeSRV:
		for _, srv := range r.SRV {
			values = append(values, srv.String())
		}
	}
	return values
}

// RecordType represents the DNS record type.
type RecordType string

// RecordType values.
const (
	RecordTypeUnknown RecordType = "Unknown"
	RecordTypeA       RecordType = "A"
	RecordTypeAAAA    RecordType = "AAAA"
	RecordTypeTXT     RecordType = "TXT"
	RecordTypeCName   RecordType = "CNAME"
	RecordTypeNS      RecordType = "NS"
	RecordTypeMX      RecordType = "MX"
	RecordTypeSRV     RecordType = "SRV"
)

// MX mail server record.
type MX struct {
	Priority int    `json:"priority"`
	Host     string `json:"host"`
}

func (mx MX) String() string {
	return fmt.Sprintf("%d %s", mx.Priority, mx.Host)
}

// SRV record.
type SRV struct {
	Priority int    `json:"priority"`
	Weight   int    `json:"weight"`
	Port     int    `json:"port"`
	Host     string `json:"host"`
}

func (srv SRV) String() string {
	return fmt.Sprintf("%d %d %d %s", srv.Priority, srv.Weight, srv.Port, srv.Host)
}

type RecordSetConfig struct {
	// DNS_NAME that this record belongs to.
	// must be fully qualified.
	// must belong to a existing Zone object.
	DNSName string `json:"dnsName"`
	// TTL of the DNS entry.
	TTL metav1.Duration `json:"ttl"`

	// Type of the RecordSet.
	// +kubebuilder:validation:Enum=A;AAAA;TXT;CNAME;NS;MX;SRV
	Type RecordType `json:"type"`
	// A record, list of IPv4 addresses.
	A []string `json:"a,omitempty"`
	// AAAA record, list of IPv6 addresses.
	AAAA []string `json:"aaaa,omitempty"`
	// TXT record, list of strings.
	TXT []string `json:"txt,omitempty"`
	// CNAME record, Canonical Name of DNSName.
	CName *string `json:"cname,omitempty"`
	// NS record, list of domain names.
	NS []string `json:"ns,omitempty"`
	// MX record, list of MX records.
	MX []MX `json:"mx,omitempty"`
	// SRV record, list of SRV records.
	SRV []SRV `json:"srv,omitempty"`
}

// RecordSet is the Schema for the recordsets API
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="DNS Name",type="string",JSONPath=".recordSet.dnsName"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".recordSet.type"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type RecordSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	RecordSetConfig `json:"recordSet"`
}

// RecordSetList contains a list of RecordSet
// +kubebuilder:object:root=true
type RecordSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RecordSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RecordSet{}, &RecordSetList{})
}
