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

// Package fssclient - FSS Connect REST API interface.
package fssclient

import (
	"encoding/json"
	"strings"
)

// LoginResponse in FSS Connect API
type LoginResponse struct {
	AccessToken      string `json:"access_token"`
	IDToken          string `json:"id_token"`
	SessionState     string `json:"session_state"`
	Scope            string `json:"scope"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	NotBeforePolicy  int    `json:"not-before-policy"`
}

// Plugins store plugins
type Plugins []Plugin

// Plugin is FSS Connect API Data Type
type Plugin struct {
	ConnectType            string `json:"connectType"`
	Name                   string `json:"name"`
	SupportsNewDeployments bool   `json:"supportsNewDeployments"`
	ID                     string `json:"id"`
	/*
		ExternalID             string `json:"externalId",omitempty`
		APIKey                 string `json:"apiKey",omitempty`
		CallbackURL            string `json:"callbackUrl",omitempty`
		PossibleSettings       []struct {
			Description string `json:"description"`
			Example     string `json:"example"`
			Name        string `json:"name"`
			Required    bool   `json:"required"`
			Unique      bool   `json:"unique"`
			Encrypted   bool   `json:"encrypted"`
		} `json:"possibleSettings",omitempty`
	*/
}

// Deployments store deployments
type Deployments []Deployment

// Deployment is FSS Connect API Data Type
type Deployment struct {
	AdminUp  bool   `json:"adminUp"`
	Name     string `json:"name"`
	PluginID string `json:"pluginID"`
	RegionID string `json:"regionId"`
	ID       string `json:"id"`
	Status   string `json:"status"`
	/*
		ExternalID  string `json:"externalId",omitempty`
		Description string `json:"description",omitempty`
		Settings    struct {
			Password string `json:"password"`
			Username string `json:"username"`
		} `json:"settings",omitempty`
	*/
}

// Tenants store tenants
type Tenants []Tenant

// Tenant is FSS Connect API Data Type
type Tenant struct {
	DeploymentID        string `json:"deploymentId"`
	FssWorkloadEvpnID   string `json:"fssWorkloadEvpnId"`
	FssWorkloadEvpnName string `json:"fssWorkloadEvpnName"`
	Name                string `json:"name"`
	FssManaged          bool   `json:"fssManaged"`
	ID                  string `json:"id"`
	Status              string `json:"status"`
	/*
		ExternalID        string `json:"externalId",omitempty`
		DeployedVersion   int    `json:"deployedVersion",omitempty`
		Version           int    `json:"version",omitempty`
	*/
}

// Subnets store subnets
type Subnets []Subnet

// Subnet is FSS Connect API Data Type
type Subnet struct {
	DeploymentID  string `json:"deploymentId"`
	TenantID      string `json:"tenantId"`
	FssSubnetID   string `json:"fssSubnetId"`
	FssSubnetName string `json:"fssSubnetName"`
	Name          string `json:"name"`
	FssManaged    bool   `json:"fssManaged"`
	ID            string `json:"id"`
	Status        string `json:"status"`
	/*
		ExternalID      string `json:"externalId",omitempty`
		DeployedVersion int    `json:"deployedVersion",omitempty`
		Version         int    `json:"version",omitempty`
	*/
}

// HostPortLabels store host port labels
type HostPortLabels []HostPortLabel

// HostPortLabel is FSS Connect API Data Type
type HostPortLabel struct {
	DeploymentID string `json:"deploymentId"`
	Name         string `json:"name"`
	ID           string `json:"id"`
	Status       string `json:"status"`
	/*
		ExternalID      string `json:"externalId",omitempty`
		DeployedVersion int    `json:"deployedVersion",omitempty`
		Version         int    `json:"version",omitempty`
	*/
}

// SubnetAssociations store subnet associations
type SubnetAssociations []SubnetAssociation

// SubnetAssociation is FSS Connect API Data Type
type SubnetAssociation struct {
	DeploymentID    string `json:"deploymentId"`
	HostPortLabelID string `json:"hostPortLabelID"`
	SubnetID        string `json:"subnetId"`
	VlanType        string `json:"vlanType"`
	VlanValue       string `json:"vlanValue"`
	ID              string `json:"id"`
	Status          string `json:"status"`
	/*
		ExternalID      string `json:"externalId",omitempty`
		DeployedVersion int    `json:"deployedVersion",omitempty`
		Version         int    `json:"version",omitempty`
	*/
}

// HostPorts store host port
type HostPorts []HostPort

// HostPort is FSS Connect API Data Type
type HostPort struct {
	DeploymentID     string   `json:"deploymentId"`
	HostName         string   `json:"hostName"`
	PortName         string   `json:"portName"`
	Name             string   `json:"name"`
	ID               string   `json:"id"`
	MacAddress       string   `json:"macAddress"`
	IsLag            bool     `json:"isLag"`
	ParentHostPortID string   `json:"parentHostPortId"`
	Status           string   `json:"status"`
	EdgeMapIds       []string `json:"edgeMapIds"`
	/*
		ExternalID       string   `json:"externalId",omitempty`
		DeployedVersion  int      `json:"deployedVersion",omitempty`
		Version          int      `json:"version",omitempty`
	*/
}

// HostPortAssociations store host port associations
type HostPortAssociations []HostPortAssociation

// HostPortAssociation is FSS Connection API Data Type
type HostPortAssociation struct {
	DeploymentID    string `json:"deploymentId"`
	HostPortID      string `json:"hostPortId"`
	HostPortLabelID string `json:"hostPortLabelId"`
	ID              string `json:"id"`
	Status          string `json:"status"`
	/*
		ExternalID      string `json:"externalId",omitempty`
		DeployedVersion int    `json:"deployedVersion",omitempty`
		Version         int    `json:"version",omitempty`
	*/
}

// ErrorResponse in FSS Connect API
type ErrorResponse struct {
	AdditionalInfo string   `json:"additional_info"`
	Detail         string   `json:"detail"`
	Errors         []string `json:"errors"`
	ObjectRef      string   `json:"object_ref"`
	Status         int      `json:"status"`
	Title          string   `json:"title"`
	Type           string   `json:"type"`
}

// BulkItem in FSS Connect Bulk API BulkResponse
type BulkItem struct {
        Status         int         `json:"status"`
        Data           interface{} `json:"data"`
}

// BulkResposeMetadata in FSS Connect Bulk API top layer
type BulkResposeMetadata struct {
        Success        int         `json:"success"`
        Failure        int         `json:"failure"`
        Total          int         `json:"total"`
}

// BulkResponse in FSS Connect Bulk API
type BulkResponse struct {
        ResponseMetadata BulkResposeMetadata `json:"responseMetadata"`
        Responses        []BulkItem          `json:"response"`
}

// Vlan is FSS Connect API Data Type
type Vlan struct {
	vlanType  string
	vlanValue string
}

// HostPortLabelIDByVlan stores host port label ID by vlan
type HostPortLabelIDByVlan map[Vlan]string

// HostPortIDByName stores host port ID by name
type HostPortIDByName map[string]string

// HostPortAssociationIDByPort stores host port association ID by port
type HostPortAssociationIDByPort map[string]string

// Database defines the data model
type Database struct {
	// Tenants by fssWorkloadEvpnId
	tenants map[string]Tenant
	// Subnets by fssSubnetId
	subnets map[string]Subnet
	// HostPortLabelID by fssSubnetId and Vlan
	hostPortLabels map[string]HostPortLabelIDByVlan
	// HostPortLabelID by fssSubnetId and Vlan
	attachedLabels map[string]HostPortLabelIDByVlan
	// HostPortID by HostName and PortName
	hostPorts map[string]HostPortIDByName
	// HostPortAssociationIDs by HostPortLabelID and HostPortID
	attachedPorts map[string][]HostPortAssociationIDByPort
	// mapping from fssWorkloadEvpnName to fssWorkloadEvpnId
	workloadMapping map[string]string
	// mapping from fssSubnetName to fssSubnetId (indexed by fssWorkloadEvpnId)
	subnetMapping map[string]map[string]string
}

// EncodedDatabase defines JSON encoded data model
type EncodedDatabase struct {
	Tenants         map[string]map[string]interface{}
	Subnets         map[string]map[string]interface{}
	HostPortLabels  map[string]map[string]string
	AttachedLabels  map[string]map[string]string
	HostPorts       map[string]HostPortIDByName
	AttachedPorts   map[string][]HostPortAssociationIDByPort
	WorkloadMapping map[string]string
	SubnetMapping   map[string]map[string]string
}

func (d *Database) encode() ([]byte, error) {
	var encoded EncodedDatabase
	encoded.Tenants = make(map[string]map[string]interface{})
	encoded.Subnets = make(map[string]map[string]interface{})
	encoded.HostPortLabels = make(map[string]map[string]string)
	encoded.AttachedLabels = make(map[string]map[string]string)
	// tenants
	for k, v := range d.tenants {
		encoded.Tenants[k] = make(map[string]interface{})
		tmp1, _ := json.Marshal(v)
		var tmp2 map[string]interface{}
		json.Unmarshal(tmp1, &tmp2)
		encoded.Tenants[k] = tmp2
	}
	// subnets
	for k, v := range d.subnets {
		encoded.Subnets[k] = make(map[string]interface{})
		tmp1, _ := json.Marshal(v)
		var tmp2 map[string]interface{}
		json.Unmarshal(tmp1, &tmp2)
		encoded.Subnets[k] = tmp2
	}
	// hostPortLabels
	for k, v := range d.hostPortLabels {
		var tmpPortLabels map[string]string
		tmpPortLabels = make(map[string]string)
		for mk, mv := range v {
			tmpPortLabels[mk.vlanType+"-"+mk.vlanValue] = mv
		}
		encoded.HostPortLabels[k] = tmpPortLabels
	}
	// attachedabels
	for k, v := range d.attachedLabels {
		var tmpPortLabels map[string]string
		tmpPortLabels = make(map[string]string)
		for mk, mv := range v {
			tmpPortLabels[mk.vlanType+"-"+mk.vlanValue] = mv
		}
		encoded.AttachedLabels[k] = tmpPortLabels
	}
	encoded.HostPorts = d.hostPorts
	encoded.AttachedPorts = d.attachedPorts
	encoded.WorkloadMapping = d.workloadMapping
	encoded.SubnetMapping = d.subnetMapping
	jsonString, err := json.Marshal(encoded)
	return jsonString, err
}

func (d *Database) decode(jsonString []byte) (Database, error) {
	var decoded Database
	decoded.tenants = make(map[string]Tenant)
	decoded.subnets = make(map[string]Subnet)
	decoded.hostPortLabels = make(map[string]HostPortLabelIDByVlan)
	decoded.attachedLabels = make(map[string]HostPortLabelIDByVlan)
	var encoded EncodedDatabase
	err := json.Unmarshal(jsonString, &encoded)
	if err != nil {
		return decoded, err
	}
	// tenants
	for k, v := range encoded.Tenants {
		tmp, err := json.Marshal(v)
		if err != nil {
			return decoded, err
		}
		var tenant Tenant
		err = json.Unmarshal(tmp, &tenant)
		if err != nil {
			return decoded, err
		}
		decoded.tenants[k] = tenant
	}
	// subports
	for k, v := range encoded.Subnets {
		tmp, err := json.Marshal(v)
		if err != nil {
			return decoded, err
		}
		var subnet Subnet
		err = json.Unmarshal(tmp, &subnet)
		if err != nil {
			return decoded, err
		}
		decoded.subnets[k] = subnet
	}
	// hostPortLabels
	for k, v := range encoded.HostPortLabels {
		var tmpPortLabels HostPortLabelIDByVlan
		tmpPortLabels = make(HostPortLabelIDByVlan)
		for mk, mv := range v {
			vlan := Vlan{strings.Split(mk, "-")[0], strings.Split(mk, "-")[1]}
			tmpPortLabels[vlan] = mv
		}
		decoded.hostPortLabels[k] = tmpPortLabels
	}
	// attachedLabels
	for k, v := range encoded.AttachedLabels {
		var tmpPortLabels HostPortLabelIDByVlan
		tmpPortLabels = make(HostPortLabelIDByVlan)
		for mk, mv := range v {
			vlan := Vlan{strings.Split(mk, "-")[0], strings.Split(mk, "-")[1]}
			tmpPortLabels[vlan] = mv
		}
		decoded.attachedLabels[k] = tmpPortLabels
	}
	decoded.hostPorts = encoded.HostPorts
	decoded.attachedPorts = encoded.AttachedPorts

	decoded.workloadMapping = make(map[string]string)
	for k, v := range encoded.WorkloadMapping {
		decoded.workloadMapping[k] = v
	}

	decoded.subnetMapping = make(map[string]map[string]string)
	for k, v := range encoded.SubnetMapping {
		decoded.subnetMapping[k] = v
	}

	return decoded, nil
}
