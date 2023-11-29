// Copyright (c) 2021 Nokia Networks
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package vlanprovider implements NCS FSS Operator backend interface.
package vlanprovider

import (
	"fmt"

	"k8s.io/client-go/kubernetes"

	"github.com/nokia/net-attach-def-admission-controller/pkg/datatypes"
)

type VlanProvider interface {
	Connect(kubernetes.Interface, string) error
	UpdateNodeTopology(string, string) (string, error)
	Attach(string, string, string, map[string]datatypes.NodeTopology, datatypes.NadAction) (map[string]error, error)
	Detach(string, string, string, map[string]datatypes.NodeTopology, datatypes.NadAction) (map[string]error, error)
	DetachNode(string)
	TxnDone()
}

func NewVlanProvider(provider string, config string) (VlanProvider, error) {
	switch provider {
	case "openstack":
		{
			openstack := &OpenstackVlanProvider{
				configFile: config}
			return openstack, nil
		}
	case "baremetal":
		{
			fss := &FssVlanProvider{
				configFile: config}
			return fss, nil
		}
	default:
		return nil, fmt.Errorf("Not supported provider: %q", provider)
	}

}
