// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DNSServer) DeepCopyInto(out *DNSServer) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DNSServer.
func (in *DNSServer) DeepCopy() *DNSServer {
	if in == nil {
		return nil
	}
	out := new(DNSServer)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DNSServer) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DNSServerList) DeepCopyInto(out *DNSServerList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DNSServer, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DNSServerList.
func (in *DNSServerList) DeepCopy() *DNSServerList {
	if in == nil {
		return nil
	}
	out := new(DNSServerList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DNSServerList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DNSServerSpec) DeepCopyInto(out *DNSServerSpec) {
	*out = *in
	out.NetworkAttachmentDefinition = in.NetworkAttachmentDefinition
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DNSServerSpec.
func (in *DNSServerSpec) DeepCopy() *DNSServerSpec {
	if in == nil {
		return nil
	}
	out := new(DNSServerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DNSServerStatus) DeepCopyInto(out *DNSServerStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DNSServerStatus.
func (in *DNSServerStatus) DeepCopy() *DNSServerStatus {
	if in == nil {
		return nil
	}
	out := new(DNSServerStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LocalObjectReference) DeepCopyInto(out *LocalObjectReference) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LocalObjectReference.
func (in *LocalObjectReference) DeepCopy() *LocalObjectReference {
	if in == nil {
		return nil
	}
	out := new(LocalObjectReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MX) DeepCopyInto(out *MX) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MX.
func (in *MX) DeepCopy() *MX {
	if in == nil {
		return nil
	}
	out := new(MX)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RecordSet) DeepCopyInto(out *RecordSet) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.RecordSetConfig.DeepCopyInto(&out.RecordSetConfig)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RecordSet.
func (in *RecordSet) DeepCopy() *RecordSet {
	if in == nil {
		return nil
	}
	out := new(RecordSet)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RecordSet) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RecordSetConfig) DeepCopyInto(out *RecordSetConfig) {
	*out = *in
	out.TTL = in.TTL
	if in.A != nil {
		in, out := &in.A, &out.A
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.AAAA != nil {
		in, out := &in.AAAA, &out.AAAA
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.TXT != nil {
		in, out := &in.TXT, &out.TXT
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.CName != nil {
		in, out := &in.CName, &out.CName
		*out = new(string)
		**out = **in
	}
	if in.NS != nil {
		in, out := &in.NS, &out.NS
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.MX != nil {
		in, out := &in.MX, &out.MX
		*out = make([]MX, len(*in))
		copy(*out, *in)
	}
	if in.SRV != nil {
		in, out := &in.SRV, &out.SRV
		*out = make([]SRV, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RecordSetConfig.
func (in *RecordSetConfig) DeepCopy() *RecordSetConfig {
	if in == nil {
		return nil
	}
	out := new(RecordSetConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RecordSetList) DeepCopyInto(out *RecordSetList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RecordSet, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RecordSetList.
func (in *RecordSetList) DeepCopy() *RecordSetList {
	if in == nil {
		return nil
	}
	out := new(RecordSetList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RecordSetList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SOARecord) DeepCopyInto(out *SOARecord) {
	*out = *in
	out.TTL = in.TTL
	out.Refresh = in.Refresh
	out.Retry = in.Retry
	out.Expire = in.Expire
	out.NegativeTTL = in.NegativeTTL
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SOARecord.
func (in *SOARecord) DeepCopy() *SOARecord {
	if in == nil {
		return nil
	}
	out := new(SOARecord)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SRV) DeepCopyInto(out *SRV) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SRV.
func (in *SRV) DeepCopy() *SRV {
	if in == nil {
		return nil
	}
	out := new(SRV)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Zone) DeepCopyInto(out *Zone) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.ZoneConfig = in.ZoneConfig
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Zone.
func (in *Zone) DeepCopy() *Zone {
	if in == nil {
		return nil
	}
	out := new(Zone)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Zone) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ZoneConfig) DeepCopyInto(out *ZoneConfig) {
	*out = *in
	out.SOA = in.SOA
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ZoneConfig.
func (in *ZoneConfig) DeepCopy() *ZoneConfig {
	if in == nil {
		return nil
	}
	out := new(ZoneConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ZoneList) DeepCopyInto(out *ZoneList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Zone, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ZoneList.
func (in *ZoneList) DeepCopy() *ZoneList {
	if in == nil {
		return nil
	}
	out := new(ZoneList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ZoneList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}