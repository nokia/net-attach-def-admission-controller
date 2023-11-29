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

// This implements NCS FSS Operator Openstack interface.
package vlanprovider

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/attachinterfaces"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/provider"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/trunks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/pagination"

	"github.com/nokia/net-attach-def-admission-controller/pkg/datatypes"
	client "github.com/nokia/net-attach-def-admission-controller/pkg/openstackclient"
	gcfg "gopkg.in/gcfg.v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

type VNic struct {
	Name       string
	MacAddress string
	TrunkID    string `json:"trunk-id,omitempty"`
	Network    string `json:"network-name,omitempty"`
	Physnet    string `json:"physnet-name,omitempty"`
}

// userAgentData is used to add extra information to the gophercloud user-agent
var userAgentData []string

// CloudConfig is used to read and store information from the cloud configuration file
type CloudConfig struct {
	Global client.AuthOpts
}

type OpenstackVlanProvider struct {
	configFile string
	epOpts     *gophercloud.EndpointOpts
	compute    *gophercloud.ServiceClient
	network    *gophercloud.ServiceClient
}

func (p *OpenstackVlanProvider) Connect(kubernetes.Interface, string) error {
	// Read Cloud Config
	f, err := os.Open(p.configFile)
	if err != nil {
		return err
	}
	defer f.Close()
	var config io.Reader
	config = f
	var cfg CloudConfig
	err = gcfg.FatalOnly(gcfg.ReadInto(&cfg, config))
	if err != nil {
		return err
	}
	// Connect to Openstack
	openstackClient, err := client.NewOpenStackClient(&cfg.Global, "ncs-openstack-sriov", userAgentData...)
	if err != nil {
		return err
	}
	p.epOpts = &gophercloud.EndpointOpts{
		Region:       cfg.Global.Region,
		Availability: cfg.Global.EndpointType,
	}
	p.compute, err = client.NewComputeV2(openstackClient, p.epOpts)
	if err != nil {
		return err
	}
	p.network, err = client.NewNetworkV2(openstackClient, p.epOpts)
	if err != nil {
		return err
	}
	klog.Info("Openstack: connected")
	return nil
}

func (p *OpenstackVlanProvider) UpdateNodeTopology(name string, topology string) (string, error) {
	// Read in node topology from node agent
	var nodeTopology datatypes.NodeTopology
	err := json.Unmarshal([]byte(topology), &nodeTopology)
	if err != nil {
		return topology, err
	}
	// Check if already updated
	for _, nic := range nodeTopology.Bonds["tenant-bond"].Ports {
		if _, ok := nic["network"]; ok {
			return topology, nil
		}
	}

	// Get nova server
	var s []servers.Server
	serverList := make([]servers.Server, 0, 1)
	opts := servers.ListOpts{
		Name: name,
	}
	pager := servers.List(p.compute, opts)
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		if err := servers.ExtractServersInto(page, &s); err != nil {
			return false, err
		}
		serverList = append(serverList, s...)
		if len(serverList) > 1 {
			return false, fmt.Errorf("Found multiple servers by name %s", name)
		}
		return true, nil
	})
	if len(serverList) == 0 {
		return topology, fmt.Errorf("No server found by name %s", name)
	}

	// Get attached ports
	var interfaces []attachinterfaces.Interface

	pager = attachinterfaces.List(p.compute, serverList[0].ID)
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		s, err := attachinterfaces.ExtractInterfaces(page)
		if err != nil {
			return false, fmt.Errorf("Failed to get interfaces for %s", name)
		}
		interfaces = append(interfaces, s...)
		return true, nil
	})

	// Get trunks
	listOpts := trunks.ListOpts{}
	allPages, err := trunks.List(p.network, listOpts).AllPages()
	if err != nil {
		return topology, err
	}
	allTrunks, err := trunks.ExtractTrunks(allPages)
	if err != nil {
		return topology, err
	}

	// Update node topology
	for _, iface := range interfaces {
		port, err := ports.Get(p.network, iface.PortID).Extract()
		if err != nil {
			return topology, err
		}
		var net struct {
			networks.Network
			provider.NetworkProviderExt
		}
		err = networks.Get(p.network, port.NetworkID).ExtractInto(&net)
		if err != nil {
			return topology, err
		}
		for _, trunk := range allTrunks {
			if iface.PortID == trunk.PortID {
				if nic, ok := nodeTopology.Bonds["tenant-bond"].Ports[iface.MACAddr]; ok {
					nic["trunk-id"] = trunk.ID
					nic["network"] = net.Name
					nic["physnet"] = net.PhysicalNetwork
					nodeTopology.Bonds["tenant-bond"].Ports[iface.MACAddr] = nic
				} else if nic, ok := nodeTopology.SriovPools[net.Name][iface.MACAddr]; ok {
					nic["trunk-id"] = trunk.ID
					nic["network"] = net.Name
					nic["physnet"] = net.PhysicalNetwork
					nodeTopology.SriovPools[net.Name][iface.MACAddr] = nic
				} else { // vfio
					for poolName := range nodeTopology.SriovPools {
						if strings.Contains(poolName, net.Name) {
							if nic, ok := nodeTopology.SriovPools[poolName][iface.MACAddr]; ok {
								nic["trunk-id"] = trunk.ID
								nic["network"] = net.Name
								nic["physnet"] = net.PhysicalNetwork
								nodeTopology.SriovPools[poolName][iface.MACAddr] = nic
							}
						}
					}
				}
			}
		}
	}
	updated, err := json.Marshal(nodeTopology)
	if err != nil {
		return topology, err
	}
	return string(updated), nil
}

func (p *OpenstackVlanProvider) Attach(project, network, vlanRange string, nodesInfo map[string]datatypes.NodeTopology, requestType datatypes.NadAction) (map[string]error, error) {
	nodesStatus := make(map[string]error)
	return nodesStatus, nil
}

func (p *OpenstackVlanProvider) Detach(project, network, vlanRange string, nodesInfo map[string]datatypes.NodeTopology, requestType datatypes.NadAction) (map[string]error, error) {
	nodesStatus := make(map[string]error)
	return nodesStatus, nil
}

func (p *OpenstackVlanProvider) DetachNode(nodeName string) {
}

func (p *OpenstackVlanProvider) TxnDone() {
}
