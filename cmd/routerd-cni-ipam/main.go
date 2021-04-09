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

// CNI IPAM Plugin for routerd using the ipam API.
package main

import (
	"encoding/json"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
)

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("static"))
}

func cmdAdd(args *skel.CmdArgs) error {
	n, err := loadConfig(args.StdinData)
	if err != nil {
		return err
	}

	result := &current.Result{
		CNIVersion: n.CNIVersion,
		DNS:        n.IPAM.DNS,
		Routes:     n.IPAM.Routes,
	}

	// Get IPs

	return types.PrintResult(result, n.CNIVersion)
}

func cmdCheck(args *skel.CmdArgs) error {
	return nil
}

func cmdDel(args *skel.CmdArgs) error {
	return nil
}

// The top-level network config - IPAM plugins are passed the full configuration
// of the calling plugin, not just the IPAM section.
type Net struct {
	Name       string     `json:"name"`
	CNIVersion string     `json:"cniVersion"`
	IPAM       IPAMConfig `json:"ipam"`
}

type IPAMConfig struct {
	Type     string         `json:"type"`
	IPv4Pool string         `json:"ipv4Pool,omitempty"`
	IPv6Pool string         `json:"ipv6Pool,omitempty"`
	Routes   []*types.Route `json:"routes"`
	DNS      types.DNS      `json:"dns"`
}

func loadConfig(bytes []byte) (*Net, error) {
	n := &Net{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, err
	}
	return n, nil
}
