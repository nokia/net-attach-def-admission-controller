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

// Package datatypes defines shared data structures
package datatypes

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	netattachdef "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
)

const (
	// NetworkTopologyKey for NCS network topology in NODE
	NetworkTopologyKey = "nokia.com/network-topology"
	// SriovResourceKey for SRIOV resource pool name in NAD
	SriovResourceKey = "k8s.v1.cni.cncf.io/resourceName"
	// NodeSelectorKey for nodeSelector in NAD
	NodeSelectorKey = "k8s.v1.cni.cncf.io/nodeSelector"
	// ExtProjectNameKey for FSS workload in NAD
	ExtProjectNameKey = "nokia.com/extProjectName"
	// ExtNetworkNameKey for FSS subnet in NAD
	ExtNetworkNameKey = "nokia.com/extNetworkName"
	// SriovOverlaysKey for SRIOV trunk in NAD
	SriovOverlaysKey = "nokia.com/sriov-vf-vlan-trunk-overlays"
	// NetworkStatusKey for NCS network status in NAD
	NetworkStatusKey = "nokia.com/network-status"
)

// Nic for NIC port
type Nic struct {
	Name       string `json:"name"`
	MacAddress string `json:"mac-address"`
}

// JSONNic for JSON encode and decode
type JSONNic map[string]interface{}

// NicMap for NIC by a primary key
type NicMap map[string]JSONNic

// Bond for NIC bond
type Bond struct {
	Mode       string `json:"mode"`
	MacAddress string `json:"mac-address"`
	Ports      NicMap
}

// NodeTopology for NIC bonds and SRIOV pools
type NodeTopology struct {
	Bonds      map[string]Bond
	SriovPools map[string]NicMap
}

// NetConf for NCS version of NetConf
type NetConf struct {
	types.NetConf
	Master    string `json:"master,omitempty"`
	Vlan      int    `json:"vlan,omitempty"`
	VlanTrunk string `json:"vlan_trunk,omitempty"`
}

// NadAction for code readability
type NadAction int

const (
	//Create ... NAD created ==> create host interface
	Create NadAction = 1
	//Delete ... NAD deleted ==> delete host interface
	Delete NadAction = 2
	//CreateAttach ... NAD created ==> open vlan on switch
	CreateAttach NadAction = 3
	//DeleteDetach ... NAD deleted ==> close vlan on switch
	DeleteDetach NadAction = 4
	//UpdateAttachDetach ... NAD updated ==> nodeSelector changed
	UpdateAttachDetach NadAction = 5
	//UpdateAttach ... NAD updated ==> becomes in scope (optional content)
	UpdateAttach NadAction = 6
	//UpdateDetach ... NAD updated ==> becomes out of scope (optional content)
	UpdateDetach NadAction = 7
	//NodeAttach ... open vlan on switch
	NodeAttach NadAction = 8
	//NodeDetach ... close vlan on switch
	NodeDetach NadAction = 9
	//NodeAttachDetach ... nodes using vlan changed
	NodeAttachDetach NadAction = 10
)

// GetVlanIds returns an array of vlanIDs
func GetVlanIds(vlanTrunk string) ([]int, error) {
	result := []int{}
	err := fmt.Errorf("Trunk format is invalid, it should follow this pattern 50,51,700-710")
	if !regexp.MustCompile(`^[0-9\,\-]*$`).MatchString(vlanTrunk) {
		return result, err
	}
	m := strings.Split(vlanTrunk, ",")
	for _, v := range m {
		if strings.Contains(v, "-") {
			n := strings.Split(v, "-")
			if len(n) != 2 {
				return result, err
			}
			min, _ := strconv.Atoi(n[0])
			max, _ := strconv.Atoi(n[1])
			if min == 0 || min > max {
				return result, err
			}
			count := max - min + 1
			for i := 0; i < count; i++ {
				result = append(result, min+i)
			}
		} else {
			vi, _ := strconv.Atoi(v)
			result = append(result, vi)
		}
	}
	return result, nil
}

// GetNetConf returns NCS version of NetConf
func GetNetConf(nad *netattachdef.NetworkAttachmentDefinition) (NetConf, error) {
	// Read NAD Config
	var netConf NetConf
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(nad.Spec.Config), &config); err != nil {
		return netConf, fmt.Errorf("read NAD config failed: %s", err.Error())
	}

	// Check if CNI config has plugin
	if p, ok := config["plugins"]; ok {
		plugins := p.([]interface{})
		for _, v := range plugins {
			plugin := v.(map[string]interface{})
			if plugin["type"] == "ipvlan" || plugin["type"] == "sriov" {
				confBytes, _ := json.Marshal(v)
				json.Unmarshal(confBytes, &netConf)
				break
			}
		}
	} else {
		json.Unmarshal([]byte(nad.Spec.Config), &netConf)
	}
	return netConf, nil
}

// ShouldTriggerTopoAction tells if NAD requires FSS Operator
func ShouldTriggerTopoAction(nad *netattachdef.NetworkAttachmentDefinition) (NetConf, bool, error) {
	// Get NAD Config
	netConf, err := GetNetConf(nad)
	if err != nil {
		return netConf, false, err
	}

	if netConf.Type != "ipvlan" && netConf.Type != "sriov" {
		return netConf, false, nil
	}

	// Check nodeSelector
	annotationsMap := nad.GetAnnotations()
	ns, ok := annotationsMap[NodeSelectorKey]
	if !ok || len(ns) == 0 {
		return netConf, false, nil
	}
	// Check NAD type
	vlanMode := true
	switch netConf.Type {
	case "ipvlan":
		{
			if netConf.Vlan < 1 || netConf.Vlan > 4095 {
				return netConf, false, fmt.Errorf("Nokia Proprietary IPVLAN vlan field has invalid value. Valid range 1..4095")
			}
			if !strings.HasPrefix(netConf.Master, "tenant") && !strings.HasPrefix(netConf.Master, "provider") {
				return netConf, false, fmt.Errorf("Nokia Proprietary IPVLAN master field has invalid value. Valid value starts with 'tenant' or 'provider'")
			}
		}
	case "sriov":
		{
			resourceName, ok := annotationsMap[SriovResourceKey]
			if !ok || len(resourceName) == 0 {
				return netConf, false, fmt.Errorf("SRIOV NAD requires resource name")
			}
			if len(netConf.VlanTrunk) > 0 {
				vlanMode = false
			} else if netConf.Vlan < 0 || netConf.Vlan > 4095 {
				return netConf, false, fmt.Errorf("vlan value is out of bound, valid range (0..4095) ")
			}
		}
	}
	if vlanMode {
		// Check extProjectName
		project, ok := annotationsMap[ExtProjectNameKey]
		if !ok || len(project) == 0 {
			return netConf, false, nil
		}
		// Check extNetworkName
		network, ok := annotationsMap[ExtNetworkNameKey]
		if !ok || len(network) == 0 {
			return netConf, false, nil
		}
	} else {
		vlanIds, err := GetVlanIds(netConf.VlanTrunk)
		if err != nil {
			return netConf, false, fmt.Errorf("Invalid vlan_trunk in CNI: %s", err.Error())
		}
		// Check Overlays
		jsonOverlays, ok := annotationsMap[SriovOverlaysKey]
		if !ok || len(jsonOverlays) == 0 {
			return netConf, false, fmt.Errorf("Missing %s in annotations", SriovOverlaysKey)
		}
		var overlays []map[string]string
		err = json.Unmarshal([]byte(jsonOverlays), &overlays)
		if err != nil {
			return netConf, false, fmt.Errorf("Invalid %s format in annotations: %s", SriovOverlaysKey, err.Error())
		}
		var vlanRanges []string
		for _, overlay := range overlays {
			_, ok1 := overlay["extProjectName"]
			_, ok2 := overlay["extNetworkName"]
			vlanRange, ok3 := overlay["vlanRange"]
			if !ok1 || !ok2 || !ok3 {
				return netConf, false, fmt.Errorf("Invalid overlay value in %s", overlay)
			}
			_, err = GetVlanIds(vlanRange)
			if err != nil {
				return netConf, false, fmt.Errorf("Invalid vlan range %s in %s: %s", vlanRange, overlay, err.Error())
			}
			vlanRanges = append(vlanRanges, vlanRange)
		}
		vlanTrunk := strings.Join(vlanRanges, ",")
		overlayVlanIds, _ := GetVlanIds(vlanTrunk)
		sort.Ints(overlayVlanIds)
		sort.Ints(vlanIds)
		if !reflect.DeepEqual(vlanIds, overlayVlanIds) {
			return netConf, false, fmt.Errorf("Different vlan ranges found in CNI and annotations")
		}
	}
	return netConf, true, nil
}

// ShouldTriggerTopoUpdate tells if NAD UPDATE requires FSS Operator
func ShouldTriggerTopoUpdate(oldNad, newNad *netattachdef.NetworkAttachmentDefinition) (NadAction, NetConf, error) {
	// Check NAD for action
	oldNetConf, trigger1, _ := ShouldTriggerTopoAction(oldNad)
	newNetConf, trigger2, err := ShouldTriggerTopoAction(newNad)

	if err != nil {
		return 0, newNetConf, err
	}
	if !trigger1 && !trigger2 {
		return 0, newNetConf, nil
	}
	if !trigger1 && trigger2 {
		return UpdateAttach, newNetConf, nil
	}
	// Implemented but not officially supported
	if trigger1 && !trigger2 {
		return 0, newNetConf, fmt.Errorf("NAD change from FSS eligible to not eligble is not allowed")
		//return UpdateDetach, newNetConf, nil
	}
	// Handle network change
	if oldNetConf.Type != newNetConf.Type {
		return 0, newNetConf, fmt.Errorf("NAD type change is not allowed")
	}
	if oldNetConf.Vlan != newNetConf.Vlan {
		return 0, newNetConf, fmt.Errorf("NAD vlan change is not allowed")
	}
	vlanMode := true
	if len(oldNetConf.VlanTrunk) > 0 {
		vlanMode = false
	}
	anno1 := oldNad.GetAnnotations()
	anno2 := newNad.GetAnnotations()
	if newNetConf.Type == "sriov" {
		resourceName1, _ := anno1[SriovResourceKey]
		resourceName2, _ := anno2[SriovResourceKey]
		if resourceName1 != resourceName2 {
			return 0, newNetConf, fmt.Errorf("SRIOV NAD resourceName change is not allowed")
		}
	}
	if vlanMode {
		proj1, _ := anno1[ExtProjectNameKey]
		net1, _ := anno1[ExtNetworkNameKey]
		proj2, _ := anno2[ExtProjectNameKey]
		net2, _ := anno2[ExtNetworkNameKey]
		if proj1 != proj2 {
			return 0, newNetConf, fmt.Errorf("NAD project change is not allowed")
		}
		if net1 != net2 {
			return 0, newNetConf, fmt.Errorf("NAD network change is not allowed")
		}
	} else {
		if oldNetConf.VlanTrunk != newNetConf.VlanTrunk {
			vlanRange1, _ := GetVlanIds(oldNetConf.VlanTrunk)
			vlanRange2, _ := GetVlanIds(newNetConf.VlanTrunk)
			checkset := make(map[int]bool)
			for _, v := range vlanRange2 {
				checkset[v] = true
			}
			for _, v := range vlanRange1 {
				if !checkset[v] {
					return 0, newNetConf, fmt.Errorf("SRIOV NAD vlan_trunk range can only increase")
				}
			}
		}
	}
	ns1, _ := anno1[NodeSelectorKey]
	ns2, _ := anno2[NodeSelectorKey]
	if !vlanMode && oldNetConf.VlanTrunk != newNetConf.VlanTrunk {
		if ns1 != ns2 {
			return 0, newNetConf, fmt.Errorf("SRIOV NAD vlan_trunk range and nodeSelector are not allowed to change together")
		}
		return UpdateAttach, newNetConf, nil
	}
	if ns1 == ns2 {
		return 0, newNetConf, nil
	}
	return UpdateAttachDetach, newNetConf, nil
}

// AddToVlanMap adds NAD as VLAN user
func AddToVlanMap(vlanMap map[string][]string, nadName string, vlanIfName string) int {
	_, ok := vlanMap[vlanIfName]
	if !ok {
		vlanMap[vlanIfName] = append(vlanMap[vlanIfName], nadName)
		return 1
	}
	numUsers := len(vlanMap[vlanIfName])
	nadExists := false
	for _, v := range vlanMap[vlanIfName] {
		if v == nadName {
			nadExists = true
			break
		}
	}
	if !nadExists {
		vlanMap[vlanIfName] = append(vlanMap[vlanIfName], nadName)
		numUsers = numUsers + 1
	}
	return numUsers
}

// DelFromVlanMap removes NAD as VLAN user
func DelFromVlanMap(vlanMap map[string][]string, nadName string, vlanIfName string) int {
	_, ok := vlanMap[vlanIfName]
	if !ok {
		return 0
	}
	numUsers := len(vlanMap[vlanIfName])
	for k, v := range vlanMap[vlanIfName] {
		if v == nadName {
			vlanMap[vlanIfName] = append(vlanMap[vlanIfName][:k], vlanMap[vlanIfName][k+1:]...)
			numUsers = numUsers - 1
		}
	}
	if numUsers == 0 {
		delete(vlanMap, vlanIfName)
	}
	return numUsers
}
