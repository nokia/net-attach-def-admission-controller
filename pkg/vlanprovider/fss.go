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

// Package vlanprovider - FSS REST API interface
package vlanprovider

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/nokia/net-attach-def-admission-controller/pkg/datatypes"
	client "github.com/nokia/net-attach-def-admission-controller/pkg/fssclient"
	gcfg "gopkg.in/gcfg.v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

// FssConfig is used to read and store information from the FSS configuration file
type FssConfig struct {
	Global client.AuthOpts
}

// FssVlanProvider stores FSS Client Config
type FssVlanProvider struct {
	configFile string
	fssClient  *client.FssClient
}

// Connect method implemeneted by FSS Client
func (p *FssVlanProvider) Connect(k8sClientSet kubernetes.Interface, podNamespace string) error {
	// Read FSS Config
	f, err := os.Open(p.configFile)
	if err != nil {
		return err
	}
	defer f.Close()
	var fData io.Reader
	fData = f
	var fssConfig FssConfig
	fssConfig.Global.Restartmode = "resync"
	err = gcfg.FatalOnly(gcfg.ReadInto(&fssConfig, fData))
	if err != nil {
		return err
	}
	// Connect to FSS
	fssClient, err := client.NewFssClient(k8sClientSet, podNamespace, &fssConfig.Global)
	if err != nil {
		return err
	}
	p.fssClient = fssClient
	klog.Info("FSS: connected")
	return nil
}

// UpdateNodeTopology method implemeneted by FSS Client
func (p *FssVlanProvider) UpdateNodeTopology(_ string, topology string) (string, error) {
	return topology, nil
}

// Attach function input parameter NodesInfo is now a map of NodeTopology
// either nodeTopology.Bonds or nodeTopology.SriovPools will be filled based on the netConf type is IPVLAN or SRIOV net
// Attach method implemeneted by FSS Client
func (p *FssVlanProvider) Attach(fssWorkloadEvpnName, fssSubnetName, vlanRange string, nodesInfo map[string]datatypes.NodeTopology, requestType datatypes.NadAction) (map[string]error, error) {
	nodesStatus := make(map[string]error)
	for k := range nodesInfo {
		nodesStatus[k] = nil
	}
        klog.Infof("Attach step 1: create/get hostPortLabel for vlanRange %s on fssWorkloadEvpnName %s fssSubnetName %s", vlanRange, fssWorkloadEvpnName, fssSubnetName)
        fssSubnetID, err := p.fssClient.CreateSubnetInterfaces(fssWorkloadEvpnName, fssSubnetName, vlanRange)
        if err != nil {
                return nodesStatus, err
        }
        attachNodes := make(datatypes.AttachNodes)
        for nodeName, nodeTopology := range nodesInfo {
                var attachNode datatypes.AttachNode
                attachNode.AttachPorts = make(map[string]bool)
                for bondName, bond := range nodeTopology.Bonds {
                        if bond.Mode == "802.3ad" {
                                nic := datatypes.Nic{
                                        Name:       bondName,
                                        MacAddress: bond.MacAddress}
                                var tmp []byte
                                tmp, _ = json.Marshal(nic)
                                var jsonNic datatypes.JSONNic
                                json.Unmarshal(tmp, &jsonNic)
                                // create parent host port
                                parentHostPortID, err := p.fssClient.CreateHostPort(nodeName, jsonNic, true, "")
                                if err != nil {
                                        nodesStatus[nodeName] = err
                                        continue
                                }
                                for _, port := range nodeTopology.Bonds[bondName].Ports {
                                        // create slave host port
                                        _, err = p.fssClient.CreateHostPort(nodeName, port, false, parentHostPortID)
                                        if err != nil {
                                                nodesStatus[nodeName] = err
                                                continue
                                        }
                                }
                                //attachNode.AttachPorts = append(attachNode.AttachPorts, bondName)
                                attachNode.AttachPorts[bondName] = true
                                klog.Infof("Node %s add attache port %s", nodeName, bondName)
                        } else {
                                for portName, port := range nodeTopology.Bonds[bondName].Ports {
                                        _, err := p.fssClient.CreateHostPort(nodeName, port, false, "")
                                        if err != nil {
                                                nodesStatus[nodeName] = err
                                                continue
                                        }
                                        //attachNode.AttachPorts = append(attachNode.AttachPorts, portName)
                                        attachNode.AttachPorts[portName] = true
                                        klog.Infof("Node %s add attache port %s", nodeName, portName)
                                }
                        }
                }
                for _, v := range nodeTopology.SriovPools {
                        for portName, port := range v {
                                _, err := p.fssClient.CreateHostPort(nodeName, port, false, "")
                                if err != nil {
                                        nodesStatus[nodeName] = err
                                        continue
                                }
                                //attachNode.AttachPorts = append(attachNode.AttachPorts, portName)
                                attachNode.AttachPorts[portName] = true
                                klog.Infof("Node %s add attache port %s", nodeName, portName)
                        }
                }
                if len(attachNode.AttachPorts) > 0  {
                        klog.Infof("node %s has ports need to be attached", nodeName)
                        attachNodes[nodeName] = attachNode
                }
        }
        if len(attachNodes) == 0 {
                klog.Infof("No nodes need attached")
                return nodesStatus, nil
        }
        klog.Infof("Attach step 2a: attach hostPortLabel for vlanRange %s to hosts", vlanRange)
        var attachNodesStatus map[string]error
        attachNodesStatus, err = p.fssClient.AttachHostPorts(fssSubnetID, vlanRange, attachNodes)
        for nodeName, _ := range attachNodes {
                if attachNodesStatus[nodeName] != nil {
                	klog.Infof("node %s attach ports status return error %+v", nodeName, attachNodesStatus[nodeName])
                        nodesStatus[nodeName] = attachNodesStatus[nodeName]
                }
        }
        if err != nil {
                return nodesStatus, err
        }
        if requestType == datatypes.CreateAttach || requestType == datatypes.UpdateAttach {
                klog.Infof("Attach step 2: attach hostPortLabel vlanRange %s on fssSubnetID %s", vlanRange, fssSubnetID)
                err = p.fssClient.AttachSubnetInterfaces(fssSubnetID, vlanRange)
                if err != nil {
                        return nodesStatus, err
                }
        }
	return nodesStatus, nil
}

// Detach method implemeneted by FSS Client
func (p *FssVlanProvider) Detach(fssWorkloadEvpnName, fssSubnetName, vlanRange string, nodesInfo map[string]datatypes.NodeTopology, requestType datatypes.NadAction) (map[string]error, error) {
	nodesStatus := make(map[string]error)
	for nodeName := range nodesInfo {
		nodesStatus[nodeName] = nil
	}
	vlanIDs, _ := datatypes.GetVlanIds(vlanRange)
	for _, vlanID := range vlanIDs {
		klog.Infof("Detach step 1: get hostPortLabel for vlan %d on fssWorkloadEvpnName %s fssSubnetName %s", vlanID, fssWorkloadEvpnName, fssSubnetName)
		fssWorkloadEvpnID, fssSubnetID, hostPortLabelID, exists := p.fssClient.GetSubnetInterface(fssWorkloadEvpnName, fssSubnetName, vlanID)
		if !exists {
			return nodesStatus, fmt.Errorf("Reqeusted vlan %d does not exist", vlanID)
		}
		if requestType == datatypes.DeleteDetach || requestType == datatypes.UpdateDetach {
			klog.Infof("Detach step 2: delete vlan %d on fssSubnetID %s", vlanID, fssSubnetID)
			err := p.fssClient.DeleteSubnetInterface(fssWorkloadEvpnID, fssSubnetID, vlanID, hostPortLabelID, requestType)
			if err != nil {
				return nodesStatus, err
			}
		} else {
			for nodeName, nodeTopology := range nodesInfo {
				for bondName, bond := range nodeTopology.Bonds {
					if bond.Mode == "802.3ad" {
						nic := datatypes.Nic{
							Name:       bondName,
							MacAddress: bond.MacAddress}
						var tmp []byte
						tmp, _ = json.Marshal(nic)
						var jsonNic datatypes.JSONNic
						json.Unmarshal(tmp, &jsonNic)
						klog.Infof("Detach step 2a: detach vlan %d from host %s parent port %s", vlanID, nodeName, bondName)
						err := p.fssClient.DetachHostPort(hostPortLabelID, nodeName, jsonNic)
						nodesStatus[nodeName] = err
					} else {
						for portName, port := range nodeTopology.Bonds[bondName].Ports {
							klog.Infof("Detach step 2a: detach vlan %d from host %s port %s", vlanID, nodeName, portName)
							err := p.fssClient.DetachHostPort(hostPortLabelID, nodeName, port)
							nodesStatus[nodeName] = err
						}
					}
				}
				for _, v := range nodeTopology.SriovPools {
					for portName, port := range v {
						klog.Infof("Detach step 2a: detach vlan %d from host %s port %s", vlanID, nodeName, portName)
						err := p.fssClient.DetachHostPort(hostPortLabelID, nodeName, port)
						nodesStatus[nodeName] = err
					}
				}
			}
		}
	}
	return nodesStatus, nil
}

// DetachNode method implemeneted by FSS Client
func (p *FssVlanProvider) DetachNode(nodeName string) {
	p.fssClient.DetachNode(nodeName)
}

// TxnDone method implemeneted by FSS Client
func (p *FssVlanProvider) TxnDone() {
	p.fssClient.TxnDone()
}
