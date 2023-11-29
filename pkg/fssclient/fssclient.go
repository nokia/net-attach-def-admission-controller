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

// Package fssclient implements FSS REST API interface for FSS Operator.
package fssclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nokia/net-attach-def-admission-controller/pkg/datatypes"
	"k8s.io/klog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// AuthOpts is adapted from Openstack Client
type AuthOpts struct {
	AuthURL     string `gcfg:"auth-url" mapstructure:"auth-url"`
	Username    string
	Password    string
	Clustername string `gcfg:"cluster-name"`
	Restartmode string `gcfg:"restart-mode"`
	Regionid    string
	Insecure    bool
}

// FssClient defines FSS REST API Client
type FssClient struct {
	cfg                AuthOpts
	rootURL            string
	refreshURL         string
	accessTokenExpiry  time.Time
	refreshTokenExpiry time.Time
	loginResponse      LoginResponse
	k8sClientSet       kubernetes.Interface
	podNamespace       string
	configmap          *corev1.ConfigMap
	plugin             Plugin
	deployment         Deployment
	database           Database
}

const (
	pluginPath              = "/rest/connect/api/v1/plugins/plugins"
	deploymentPath          = "/rest/connect/api/v1/plugins/deployments"
	tenantPath              = "/rest/connect/api/v1/plugins/tenants"
	subnetPath              = "/rest/connect/api/v1/plugins/subnets"
	hostPortLabelPath       = "/rest/connect/api/v1/plugins/hostportlabels"
	hostPortPath            = "/rest/connect/api/v1/plugins/hostports"
	hostPortAssociationPath = "/rest/connect/api/v1/plugins/hostportlabelhostportassociations"
	subnetAssociationPath   = "/rest/connect/api/v1/plugins/hostportlabelsubnetassociations"
)

// GetAccessToken checks if access token is still valid
func (f *FssClient) GetAccessToken() error {
	now := time.Now()
	// Check if refreshToken expiried
	if now.After(f.refreshTokenExpiry) {
		klog.V(3).Info("refresh_token expired, login again")
		return f.login(f.cfg.AuthURL)
	}
	// Check if accessToken expiried
	if now.After(f.accessTokenExpiry) {
		klog.V(3).Info("access_token expired, refresh it")
		return f.login(f.refreshURL)
	}
	return nil
}

// GET implements GET method
func (f *FssClient) GET(path string) (int, []byte, error) {
	err := f.GetAccessToken()
	if err != nil {
		return 0, nil, err
	}
	u := f.rootURL + path
	request, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return 0, nil, err
	}
	request.Header.Add("Authorization", "Bearer "+f.loginResponse.AccessToken)
	client := &http.Client{}
	if f.cfg.Insecure {
		transCfg := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore SSL certificates
		}
		client.Transport = transCfg
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	jsonRespData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, nil, err
	}
	return response.StatusCode, jsonRespData, err
}

// DELETE implements DELETE method
func (f *FssClient) DELETE(path string) (int, []byte, error) {
	err := f.GetAccessToken()
	if err != nil {
		return 0, nil, err
	}
	u := f.rootURL + path
	request, err := http.NewRequest("DELETE", u, nil)
	if err != nil {
		return 0, nil, err
	}
	request.Header.Add("Authorization", "Bearer "+f.loginResponse.AccessToken)
	client := &http.Client{}
	if f.cfg.Insecure {
		transCfg := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore SSL certificates
		}
		client.Transport = transCfg
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	jsonRespData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, nil, err
	}
	return response.StatusCode, jsonRespData, err
}

// POST implements POST method
func (f *FssClient) POST(path string, jsonReqData []byte) (int, []byte, error) {
	err := f.GetAccessToken()
	if err != nil {
		return 0, nil, err
	}
	u := f.rootURL + path
	var jsonBody *bytes.Buffer
	if len(jsonReqData) > 0 {
		jsonBody = bytes.NewBuffer(jsonReqData)
	}
	request, err := http.NewRequest("POST", u, jsonBody)
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")
	request.Header.Add("Authorization", "Bearer "+f.loginResponse.AccessToken)
	client := &http.Client{}
	if f.cfg.Insecure {
		transCfg := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore SSL certificates
		}
		client.Transport = transCfg
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	jsonRespData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, nil, err
	}
	return response.StatusCode, jsonRespData, err
}

func (f *FssClient) getConfigMap(name string) []byte {
	return []byte(f.configmap.Data[name])
}

func (f *FssClient) setConfigMap(name string, data []byte) error {
	klog.V(3).Infof("Save %s to configMap fss-database", name)
	var err error
	for i := 0; i < 256; i++ {
		klog.V(3).Infof("Attempt %d", i+1)
		f.configmap, err = f.k8sClientSet.CoreV1().ConfigMaps(f.podNamespace).Get(context.TODO(), "fss-database", metav1.GetOptions{})
		f.configmap.Data[name] = string(data)
		_, err = f.k8sClientSet.CoreV1().ConfigMaps(f.podNamespace).Update(context.TODO(), f.configmap, metav1.UpdateOptions{})
		if err == nil {
			return nil
		}
		if !errors.IsConflict(err) {
			return err
		}
	}
	return err
}

// TxnDone marks end of a transaction
func (f *FssClient) TxnDone() {
	jsonString, err := f.database.encode()
	if err != nil {
		klog.Errorf("Database encoding error: %s", err.Error())
	} else {
		f.setConfigMap("database", jsonString)
	}
}

func (f *FssClient) login(loginURL string) error {
	var jsonReqData []byte
	if loginURL == f.refreshURL {
		jsonReqData, _ = json.Marshal(map[string]string{
			"refresh_token": f.loginResponse.RefreshToken,
		})
	} else {
		jsonReqData, _ = json.Marshal(map[string]string{
			"username": f.cfg.Username,
			"password": f.cfg.Password,
		})
	}
	request, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(jsonReqData))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")
	if loginURL == f.refreshURL {
		request.Header.Add("Authorization", "Bearer "+f.loginResponse.AccessToken)
	}
	client := &http.Client{}
	if f.cfg.Insecure {
		transCfg := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore SSL certificates
		}
		client.Transport = transCfg
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	jsonRespData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		var errorResponse ErrorResponse
		json.Unmarshal(jsonRespData, &errorResponse)
		klog.Errorf("Login error: %+v", errorResponse)
		return fmt.Errorf("Login failed with status=%d", response.StatusCode)
	}
	var result LoginResponse
	err = json.Unmarshal(jsonRespData, &result)
	if err != nil {
		return err
	}
	now := time.Now()
	f.accessTokenExpiry = now.Add(time.Duration(result.ExpiresIn) * time.Second)
	if loginURL != f.refreshURL {
		f.refreshTokenExpiry = now.Add(time.Duration(result.RefreshExpiresIn) * time.Second)
	}
	f.loginResponse = result
	return nil
}

// NewFssClient creates a new instance of FSS REST API Client
func NewFssClient(k8sClientSet kubernetes.Interface, podNamespace string, cfg *AuthOpts) (*FssClient, error) {
	u, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return nil, err
	}
	f := &FssClient{
		cfg:          *cfg,
		rootURL:      u.Scheme + "://" + u.Host,
		refreshURL:   strings.Replace(cfg.AuthURL, "login", "refresh", 1),
		k8sClientSet: k8sClientSet,
		podNamespace: podNamespace,
	}
	// Login
	klog.Infof("Login to FSS: %s", cfg.AuthURL)
	err = f.login(cfg.AuthURL)
	if err != nil {
		return nil, err
	}
	// Check if this is the first run
	firstRun := false
	hasDeployment := false
	f.configmap, err = k8sClientSet.CoreV1().ConfigMaps(podNamespace).Get(context.TODO(), "fss-database", metav1.GetOptions{})
	if err != nil {
		firstRun = true
		klog.Infof("Create ConfigMap fss-database")
		f.configmap = &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fss-database",
				Namespace: podNamespace,
			},
			Data: map[string]string{
				"plugin":     "",
				"deployment": "",
				"database":   "",
			},
		}
		f.configmap, err = f.k8sClientSet.CoreV1().ConfigMaps(podNamespace).Create(context.TODO(), f.configmap, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
		klog.Infof("ConfigMap fss-database created")
	}
	// Check the last registration
	if !firstRun {
		var plugin Plugin
		jsonString := f.getConfigMap("plugin")
		err = json.Unmarshal(jsonString, &plugin)
		if err == nil && len(plugin.ID) > 0 {
			klog.Infof("Plugin from last run: %+v", plugin)
			// Validate with Connect Core
			u := pluginPath + "/" + plugin.ID
			statusCode, _, err := f.GET(u)
			if err != nil {
				return nil, err
			}
			if statusCode != 200 {
				klog.Infof("Last plugin is not longer valid")
				firstRun = true
			} else {
				klog.Infof("Last plugin is still valid")
				f.plugin = plugin
			}
		} else {
			klog.Infof("No plugin found from last run")
			firstRun = true
		}
	}
	// Check the last deployment
	if !firstRun {
		var deployment Deployment
		jsonString := f.getConfigMap("deployment")
		if len(jsonString) > 0 {
			err = json.Unmarshal(jsonString, &deployment)
			if err == nil && deployment.PluginID == f.plugin.ID {
				klog.Infof("Deployment from last run: %+v", deployment)
				// Validate with Connect Core
				u := deploymentPath + "/" + deployment.ID
				statusCode, _, err := f.GET(u)
				if err != nil {
					return nil, err
				}
				if statusCode != 200 {
					klog.Infof("Last deployment is not longer valid")
				} else {
					klog.Infof("Last deployment is still valid")
					hasDeployment = true
					f.deployment = deployment
				}
			} else {
				klog.Infof("No deployment found from last run")
			}
		}
	}
	if firstRun {
		klog.Infof("Start a new run")
		// Create plugin
		f.plugin = Plugin{
			ConnectType:            "kubernetes",
			Name:                   "ncs-" + cfg.Clustername,
			SupportsNewDeployments: false,
		}
		jsonRequest, _ := json.Marshal(f.plugin)
		statusCode, jsonResponse, err := f.POST(pluginPath, jsonRequest)
		if err != nil {
			return nil, err
		}
		if statusCode != 201 {
			var errorResponse ErrorResponse
			json.Unmarshal(jsonResponse, &errorResponse)
			klog.Errorf("Plugin error: %+v", errorResponse)
			return nil, fmt.Errorf("Create plugin failed with status=%d", statusCode)
		}
		json.Unmarshal(jsonResponse, &f.plugin)
		klog.Infof("Plugin created: %+v", f.plugin)
		jsonString, _ := json.Marshal(f.plugin)
		err = f.setConfigMap("plugin", jsonString)
		if err != nil {
			return nil, err
		}
	}
	// Create deployment
	if !hasDeployment {
		f.deployment = Deployment{
			AdminUp:  false,
			Name:     "ncs-" + cfg.Clustername,
			PluginID: f.plugin.ID,
			RegionID: cfg.Regionid,
		}
		jsonRequest, _ := json.Marshal(f.deployment)
		statusCode, jsonResponse, err := f.POST(deploymentPath, jsonRequest)
		if err != nil {
			return nil, err
		}
		if statusCode != 201 {
			var errorResponse ErrorResponse
			json.Unmarshal(jsonResponse, &errorResponse)
			klog.Errorf("Deployment error: %+v", errorResponse)
			return nil, fmt.Errorf("Create deployment failed with status=%d", statusCode)
		}
		json.Unmarshal(jsonResponse, &f.deployment)
		klog.Infof("Deployment created: %+v", f.deployment)
		jsonString, _ := json.Marshal(f.deployment)
		err = f.setConfigMap("deployment", jsonString)
		if err != nil {
			return nil, err
		}
	}
	// Wait Admin set adminUp to true
	if !f.deployment.AdminUp {
		klog.Infof("Wait adminUp becomes true for plugin %s deployment %s ...", f.plugin.ID, f.deployment.ID)
		path := deploymentPath + "/" + f.deployment.ID
		for !f.deployment.AdminUp {
			time.Sleep(10 * time.Second)
			statusCode, jsonResponse, err := f.GET(path)
			if err != nil {
				return nil, err
			}
			if statusCode != 200 {
				return nil, fmt.Errorf("Get deployment failed with status=%d", statusCode)
			}
			json.Unmarshal(jsonResponse, &f.deployment)
			if f.deployment.AdminUp {
				klog.Infof("Deployment is ready: %+v", f.deployment)
				jsonString, _ := json.Marshal(f.deployment)
				err = f.setConfigMap("deployment", jsonString)
				if err != nil {
					return nil, err
				}
				break
			}
		}
	}
	// Create database
	f.database = Database{
		tenants:         make(map[string]Tenant),
		subnets:         make(map[string]Subnet),
		hostPortLabels:  make(map[string]HostPortLabelIDByVlan),
		attachedLabels:  make(map[string]HostPortLabelIDByVlan),
		hostPorts:       make(map[string]HostPortIDByName),
		attachedPorts:   make(map[string][]HostPortAssociationIDByPort),
		workloadMapping: make(map[string]string),
		subnetMapping:   make(map[string]map[string]string),
	}
	if firstRun {
		f.TxnDone()
	} else {
		klog.Infof("Load tenant data from last run")
		var database Database
		jsonString := f.getConfigMap("database")
		if len(jsonString) > 0 {
			database, err = database.decode(jsonString)
			if err != nil {
				klog.Errorf("Database decoding error: %s", err.Error())
			} else {
				f.database = database
			}
		}
	}
	if cfg.Restartmode == "resync" {
		klog.Infof("Resync tenant data with server")
		err = f.Resync(firstRun, f.deployment.ID)
		if err != nil {
			klog.Warningf("Resync with server failed: %s", err.Error())
		}
	}
	return f, nil
}

/*
Resync path: hostPortlabels, hostPorts, tenants
HostPortLabel: When deleting a HostPortLabel, the associations to Subnet and HostPort are automatically deleted.
HostPort: When deleting a HostPort, the associations to HostPortLabel are automatically deleted.
Subnet: When deleting a Subnet, the associations to HostPortLabel are automatically deleted.
Tenant: When deleting a Tenant, the subnets connected to this Tenant are automatically deleted.
*/
func (f *FssClient) Resync(firstRun bool, deploymentID string) error {
	if firstRun {
		// Upon firstRun, purge old tenant data in the server
		// This is added to faciliate testing
		deploymentName := "ncs-" + f.cfg.Clustername
		statusCode, jsonResponse, err := f.GET(deploymentPath)
		if err != nil {
			return err
		}
		if statusCode != 200 {
			return fmt.Errorf("Get deployments failed with status=%d", statusCode)
		}
		var deployments Deployments
		json.Unmarshal(jsonResponse, &deployments)
		for _, v := range deployments {
			if v.Name == deploymentName && v.ID != deploymentID {
				// delete hostPortLabels
				statusCode, jsonResponse, err := f.GET(hostPortLabelPath)
				if err != nil {
					return err
				}
				if statusCode != 200 {
					klog.Errorf("Get hostPortLabels failed with status=%d: %s", statusCode, err.Error())
				}
				var hostPortLabels HostPortLabels
				json.Unmarshal(jsonResponse, &hostPortLabels)
				for _, v1 := range hostPortLabels {
					if v.ID == v1.DeploymentID {
						u := hostPortLabelPath + "/" + v1.ID
						statusCode, _, err := f.DELETE(u)
						if err != nil {
							klog.Errorf("Delete hostPortLabel failed with status=%d: %s", statusCode, err.Error())
						}
					}
				}
				// delete hostPorts
				statusCode, jsonResponse, err = f.GET(hostPortPath)
				if err != nil {
					return err
				}
				if statusCode != 200 {
					return fmt.Errorf("Get hostPorts failed with status=%d", statusCode)
				}
				var hostPorts HostPorts
				var lagPorts = make(map[string]HostPortIDByName)
				json.Unmarshal(jsonResponse, &hostPorts)
				for _, v1 := range hostPorts {
					if v.ID == v1.DeploymentID {
						if !v1.IsLag {
							u := hostPortPath + "/" + v1.ID
							klog.Infof("Delete path=%s", u)
							statusCode, _, err := f.DELETE(u)
							if err != nil {
								klog.Errorf("Delete host %s hostPort %s failed with status=%d: %s", v1.HostName, v1.PortName, statusCode, err.Error())
							}
							if statusCode != 204 {
								klog.Errorf("Delete host %s hostPort %s failed with status=%d", v1.HostName, v1.PortName, statusCode)
							}
						} else {
							_, ok := lagPorts[v1.HostName]
							if !ok {
								lagPorts[v1.HostName] = make(HostPortIDByName)
							}
							lagPorts[v1.HostName][v1.PortName] = v1.ID
						}
					}
				}
				// delete lag host ports at last
				for nodeName, lagPortsInNode := range lagPorts {
					for lagPortName, lagPortID := range lagPortsInNode {
						u := hostPortPath + "/" + lagPortID
						klog.Infof("Delete path=%s", u)
						statusCode, _, err := f.DELETE(u)
						if err != nil {
							klog.Errorf("Delete host %s lag hostPort %s failed with status=%d: %s", nodeName, lagPortName, statusCode, err.Error())
						}
						if statusCode != 204 {
							klog.Errorf("Delete host %s lag hostPort %s failed with status=%d", nodeName, lagPortName, statusCode)
						}
					}
				}
				// delete tenants
				statusCode, jsonResponse, err = f.GET(tenantPath)
				if err != nil {
					return err
				}
				if statusCode != 200 {
					return fmt.Errorf("Get tenants failed with status=%d", statusCode)
				}
				var tenants Tenants
				json.Unmarshal(jsonResponse, &tenants)
				for _, v1 := range tenants {
					if v.ID == v1.DeploymentID {
						u := tenantPath + "/" + v1.ID
						klog.Infof("Delete path=%s", u)
						statusCode, _, err := f.DELETE(u)
						if err != nil {
							klog.Errorf("Delete tenant failed with status=%d: %s", statusCode, err.Error())
						}
					}
				}
			}
		}
		return nil
	}

	// Upon restart, purge local tenant data not existing on the server
	statusCode, jsonResponse, err := f.GET(tenantPath)
	if err != nil {
		return err
	}
	if statusCode != 200 {
		return fmt.Errorf("Get tenants failed with status=%d", statusCode)
	}
	var serverTenants Tenants
	json.Unmarshal(jsonResponse, &serverTenants)
	for fssWorkloadEvpnID, localTenant := range f.database.tenants {
		if localTenant.DeploymentID == deploymentID {
			// Check if local Tenant is known to the server
			knownObject := false
			for _, serverTenant := range serverTenants {
				if fssWorkloadEvpnID == serverTenant.FssWorkloadEvpnID {
					knownObject = true
					break
				}
			}

			// Delete unknown tenant and associated mappings
			if !knownObject {
				klog.Warningf("Delete unknown tenant for workload %s from database: %+v", fssWorkloadEvpnID, localTenant)
				delete(f.database.tenants, fssWorkloadEvpnID)
				delete(f.database.workloadMapping, localTenant.FssWorkloadEvpnName)
				delete(f.database.subnetMapping, fssWorkloadEvpnID)

				// hanging subnets will be deleted in the next step
			}
		}
	}

	statusCode, jsonResponse, err = f.GET(subnetPath)
	if err != nil {
		return err
	}
	if statusCode != 200 {
		return fmt.Errorf("Get subnets failed with status=%d", statusCode)
	}
	var serverSubnets Subnets
	json.Unmarshal(jsonResponse, &serverSubnets)
	for fssSubnetID, localSubnet := range f.database.subnets {
		if localSubnet.DeploymentID == deploymentID {
			// Check if local Subnet is known to the server
			knownObject := false
			for _, serverSubnet := range serverSubnets {
				if fssSubnetID == serverSubnet.FssSubnetID {
					knownObject = true
					break
				}
			}

			// Delete unknown subnet and associated labels and attached ports
			if !knownObject {
				klog.Warningf("Delete unknown subnet %s from database: %+v", fssSubnetID, localSubnet)
				delete(f.database.subnets, fssSubnetID)

				klog.Warningf("Delete labels and attached ports associated with subnet %s from database", fssSubnetID)
				delete(f.database.attachedLabels, fssSubnetID)

				hostPortLabelIDByVlan, exists := f.database.hostPortLabels[fssSubnetID]
				if exists {
					delete(f.database.hostPortLabels, fssSubnetID)

					for _, hostPortLabelID := range hostPortLabelIDByVlan {
						delete(f.database.attachedPorts, hostPortLabelID)
					}
				}
			}
		}
	}

	// update database with the changes
	f.TxnDone()

	// Purge unknown tenant data on the server
	// Local database contains all committed data

	// Check hostPortLabels
	statusCode, jsonResponse, err = f.GET(hostPortLabelPath)
	if err != nil {
		return err
	}
	if statusCode != 200 {
		return fmt.Errorf("Get hostPortLabels failed with status=%d", statusCode)
	}
	var hostPortLabels HostPortLabels
	json.Unmarshal(jsonResponse, &hostPortLabels)
	for _, v := range hostPortLabels {
		if v.DeploymentID != deploymentID {
			continue
		}
		// Check if object is known
		knownObject := false
		for _, v1 := range f.database.hostPortLabels {
			for _, v2 := range v1 {
				if v.ID == v2 {
					knownObject = true
					break
				}
			}
		}
		// Delete unknown object
		if !knownObject {
			u := hostPortLabelPath + "/" + v.ID
			klog.Warningf("Delete unknown hostPortLabel in server: %s", u)
			statusCode, _, err := f.DELETE(u)
			if err != nil {
				klog.Errorf("Delete hostPortLabel failed: %s", err.Error())
			}
			if statusCode != 204 {
				klog.Errorf("Delete hostPortLabel failed with status=%d", statusCode)
			}
		}
	}
	// Check hostPorts
	statusCode, jsonResponse, err = f.GET(hostPortPath)
	if err != nil {
		return err
	}
	if statusCode != 200 {
		return fmt.Errorf("Get hostPorts failed with status=%d", statusCode)
	}
	var hostPorts HostPorts
	var lagPorts = make(map[string]HostPortIDByName)
	json.Unmarshal(jsonResponse, &hostPorts)
	for _, v := range hostPorts {
		if v.DeploymentID == deploymentID {
			// Check if object is known
			knownObject := false
			for _, v1 := range f.database.hostPorts {
				for _, v2 := range v1 {
					if v.ID == v2 {
						knownObject = true
						break
					}
				}
			}
			// Delete unknown object
			if !knownObject {
				if !v.IsLag {
					u := hostPortPath + "/" + v.ID
					klog.Warningf("Delete unknown hostPort in server: %s", u)
					statusCode, _, err := f.DELETE(u)
					if err != nil {
						klog.Errorf("Delete hostPort failed: %s", err.Error())
					}
					if statusCode != 204 {
						klog.Errorf("Delete hostPort failed with status=%d", statusCode)
					}
				} else {
					_, ok := lagPorts[v.HostName]
					if !ok {
						lagPorts[v.HostName] = make(HostPortIDByName)
					}
					lagPorts[v.HostName][v.PortName] = v.ID
				}
			}
		}
	}
	// delete lag ports at the last
	for nodeName, lagPortsToDelete := range lagPorts {
		for lagPortName, lagPortID := range lagPortsToDelete {
			u := hostPortPath + "/" + lagPortID
			klog.Warningf("Delete unknown hostPort in server: %s", u)
			statusCode, _, err := f.DELETE(u)
			if err != nil {
				klog.Errorf("Delete host %s lag hostPort %s failed: %s", nodeName, lagPortName, err.Error())
			}
			if statusCode != 204 {
				klog.Errorf("Delete host %s lag hostPort %s failed with status=%d", nodeName, lagPortName, statusCode)
			}
		}
	}
	// Check tenants (they were already fetched from server in one of the previous steps)
	for _, v := range serverTenants {
		if v.DeploymentID == deploymentID {
			// Check if object is known
			knownObject := false
			for _, v1 := range f.database.tenants {
				if v.ID == v1.ID {
					knownObject = true
					break
				}
			}
			// Delete unknown object
			if !knownObject {
				u := tenantPath + "/" + v.ID
				klog.Warningf("Delete unknown tenant in server: %s", u)
				statusCode, _, err := f.DELETE(u)
				if err != nil {
					klog.Errorf("Delete tenant failed: %s", err.Error())
				}
				if statusCode != 204 {
					klog.Errorf("Delete tenant failed with status=%d", statusCode)
				}
			}
		}
	}
	return nil
}

// CreateSubnetInterface creates VLAN interface (host port label)
func (f *FssClient) CreateSubnetInterface(fssWorkloadEvpnName string, fssSubnetName string, vlanID int) (string, string, error) {
	fssSubnetID := ""
	hostPortLabelID := ""

	fssWorkloadEvpnID, ok1 := f.database.workloadMapping[fssWorkloadEvpnName]
	if !ok1 {
		// Create the tenant
		klog.Infof("Create tenant for fssWorkloadEvpnName %s", fssWorkloadEvpnName)
		tenant := Tenant{
			DeploymentID:        f.deployment.ID,
			FssWorkloadEvpnName: fssWorkloadEvpnName,
			Name:                "tenant-" + fssWorkloadEvpnName,
			FssManaged:          true,
		}
		jsonRequest, _ := json.Marshal(tenant)
		statusCode, jsonResponse, err := f.POST(tenantPath, jsonRequest)
		if err != nil {
			return fssSubnetID, hostPortLabelID, err
		}
		if statusCode != 201 {
			var errorResponse ErrorResponse
			json.Unmarshal(jsonResponse, &errorResponse)
			klog.Errorf("Tenant error: %+v", errorResponse)
			return fssSubnetID, hostPortLabelID, fmt.Errorf("Create tenant failed with status=%d", statusCode)
		}
		json.Unmarshal(jsonResponse, &tenant)
		klog.Infof("Tenant is created: %+v", tenant)
		fssWorkloadEvpnID = tenant.FssWorkloadEvpnID
		f.database.workloadMapping[fssWorkloadEvpnName] = fssWorkloadEvpnID
		f.database.subnetMapping[fssWorkloadEvpnID] = make(map[string]string)
		f.database.tenants[fssWorkloadEvpnID] = tenant
	}

	fssSubnetID, ok2 := f.database.subnetMapping[fssWorkloadEvpnID][fssSubnetName]
	if !ok2 {
		// Create the subnet
		klog.Infof("Create subnet for fssSubnetName %s", fssSubnetName)
		subnet := Subnet{
			DeploymentID:  f.deployment.ID,
			TenantID:      f.database.tenants[fssWorkloadEvpnID].ID,
			FssSubnetName: fssSubnetName,
			Name:          "subnet-" + fssSubnetName,
			FssManaged:    true,
		}
		jsonRequest, _ := json.Marshal(subnet)
		statusCode, jsonResponse, err := f.POST(subnetPath, jsonRequest)
		if err != nil {
			return fssSubnetID, hostPortLabelID, err
		}
		if statusCode != 201 {
			var errorResponse ErrorResponse
			json.Unmarshal(jsonResponse, &errorResponse)
			klog.Errorf("Subnet error: %+v", errorResponse)
			return fssSubnetID, hostPortLabelID, fmt.Errorf("Create subnet failed with status=%d", statusCode)
		}
		json.Unmarshal(jsonResponse, &subnet)
		klog.Infof("Subnet is created: %+v", subnet)
		fssSubnetID = subnet.FssSubnetID
		f.database.subnetMapping[fssWorkloadEvpnID][fssSubnetName] = fssSubnetID
		f.database.subnets[fssSubnetID] = subnet
		f.database.hostPortLabels[fssSubnetID] = make(HostPortLabelIDByVlan)
		f.database.attachedLabels[fssSubnetID] = make(HostPortLabelIDByVlan)
	}
	hostPortLabels := f.database.hostPortLabels[fssSubnetID]
	vlanType := "value"
	vlanValue := strconv.Itoa(vlanID)
	if vlanID == 0 {
		vlanType = "untagged"
		vlanValue = ""
	}
	vlan := Vlan{vlanType, vlanValue}
	hostPortLabelID, ok3 := hostPortLabels[vlan]
	if ok1 && ok2 && ok3 {
		return fssSubnetID, hostPortLabelID, nil
	}
	// Create the hostPortLabel
	klog.Infof("Create hostPortLabel for fssSubnetID %s and vlanID %d", fssSubnetID, vlanID)
	hostPortLabel := HostPortLabel{
		DeploymentID: f.deployment.ID,
		Name:         "label-" + fssSubnetID + "-" + strconv.Itoa(vlanID),
	}
	jsonRequest, _ := json.Marshal(hostPortLabel)
	statusCode, jsonResponse, err := f.POST(hostPortLabelPath, jsonRequest)
	if err != nil {
		return fssSubnetID, hostPortLabelID, err
	}
	if statusCode != 201 {
		var errorResponse ErrorResponse
		json.Unmarshal(jsonResponse, &errorResponse)
		klog.Errorf("HostPortLabel error: %+v", errorResponse)
		return fssSubnetID, hostPortLabelID, fmt.Errorf("Create hostPortLabel failed with status=%d", statusCode)
	}
	json.Unmarshal(jsonResponse, &hostPortLabel)
	klog.Infof("HostPortLabel is created: %+v", hostPortLabel)
	f.database.hostPortLabels[fssSubnetID][vlan] = hostPortLabel.ID
	return fssSubnetID, hostPortLabel.ID, nil
}

// GetSubnetInterface returns VLAN interface (host port label) if exists
func (f *FssClient) GetSubnetInterface(fssWorkloadEvpnName string, fssSubnetName string, vlanID int) (string, string, string, bool) {
	fssWorkloadEvpnID, ok := f.database.workloadMapping[fssWorkloadEvpnName]
	if !ok {
		return "", "", "", false
	}
	fssSubnetID, ok := f.database.subnetMapping[fssWorkloadEvpnID][fssSubnetName]
	if !ok {
		return fssWorkloadEvpnID, "", "", false
	}
	hostPortLabels := f.database.hostPortLabels[fssSubnetID]
	vlanType := "value"
	vlanValue := strconv.Itoa(vlanID)
	if vlanID == 0 {
		vlanType = "untagged"
		vlanValue = ""
	}
	vlan := Vlan{vlanType, vlanValue}
	hostPortLabelID, ok := hostPortLabels[vlan]
	if !ok {
		return fssWorkloadEvpnID, fssSubnetID, "", false
	}
	return fssWorkloadEvpnID, fssSubnetID, hostPortLabelID, true
}

// AttachSubnetInterface attaches VLAN interface (host port label) to subnet
func (f *FssClient) AttachSubnetInterface(fssSubnetID string, vlanID int, hostPortLabelID string) error {
	klog.Infof("Attach hostPortLabel %s to fssSubnetID %s for vlanID %d", hostPortLabelID, fssSubnetID, vlanID)
	attachedLabels := f.database.attachedLabels[fssSubnetID]
	vlanType := "value"
	vlanValue := strconv.Itoa(vlanID)
	if vlanID == 0 {
		vlanType = "untagged"
		vlanValue = ""
	}
	vlan := Vlan{vlanType, vlanValue}
	_, ok := attachedLabels[vlan]
	if ok && hostPortLabelID == attachedLabels[vlan] {
		klog.Infof("hostPortLabel %s already attached", hostPortLabelID)
		return nil
	}
	subnetAssociation := SubnetAssociation{
		DeploymentID:    f.deployment.ID,
		HostPortLabelID: hostPortLabelID,
		SubnetID:        f.database.subnets[fssSubnetID].ID,
		VlanType:        vlanType,
		VlanValue:       vlanValue,
	}
	jsonRequest, _ := json.Marshal(subnetAssociation)
	statusCode, jsonResponse, err := f.POST(subnetAssociationPath, jsonRequest)
	if err != nil {
		return err
	}
	if statusCode != 201 {
		var errorResponse ErrorResponse
		json.Unmarshal(jsonResponse, &errorResponse)
		klog.Errorf("SubnetAssociation error: %+v", errorResponse)
		return fmt.Errorf("Create SubnetAssociation failed with status=%d", statusCode)
	}
	json.Unmarshal(jsonResponse, &subnetAssociation)
	klog.Infof("SubnetAssociation is created: %+v", subnetAssociation)
	f.database.attachedLabels[fssSubnetID][vlan] = subnetAssociation.HostPortLabelID
	return nil
}

// DeleteSubnetInterface deletes VLAN interface (host port label)
func (f *FssClient) DeleteSubnetInterface(fssWorkloadEvpnID string, fssSubnetID string, vlanID int, hostPortLabelID string, requestType datatypes.NadAction) error {
	klog.Infof("Delete hostPortLabel %s for fssSubnetID %s and vlanID %d", hostPortLabelID, fssSubnetID, vlanID)
	var result error
	vlanType := "value"
	vlanValue := strconv.Itoa(vlanID)
	if vlanID == 0 {
		vlanType = "untagged"
		vlanValue = ""
	}
	vlan := Vlan{vlanType, vlanValue}
	_, ok := f.database.attachedLabels[fssSubnetID][vlan]
	if ok && hostPortLabelID == f.database.attachedLabels[fssSubnetID][vlan] {
		// HostPortLabel: When deleting a HostPortLabel, the associations to Subnet and HostPort are automatically deleted.
		u := hostPortLabelPath + "/" + hostPortLabelID
		statusCode, _, err := f.DELETE(u)
		if err != nil {
			return err
		}
		if statusCode != 204 {
			result = fmt.Errorf("Delete hostPortLabel failed with status=%d", statusCode)
		}
		klog.Infof("HostPortLabel %s is deleted", hostPortLabelID)
	} else {
		klog.Infof("HostPortLabel %s does not exists", hostPortLabelID)
	}
	// Local deletion: hostPortLabels, attacheLabels, attachedHostPorts
	delete(f.database.hostPortLabels[fssSubnetID], vlan)
	delete(f.database.attachedLabels[fssSubnetID], vlan)
	delete(f.database.attachedPorts, hostPortLabelID)

	// In order to prevent hanging resource on the FSS connect, we need to delete the subnet and tenant upon last NAD deletion:
	// The sequence flow is as follow:
	// NAD deletion -> handleNetAttachDefDeleteEvent: last NAD on the vlan: DeleteDetach -> processNadItem(DeleteDetach)
	// ->  handleNetworkDetach -> Detach -> DeleteSubnetInterface -> delete hostport label from subnet
	// when last hostport label is removed from subnet, we will remove the subnet from FSS connect
	// when last subnet is removed from tenant, we will remove the tenant from FSS connnect
	if requestType == datatypes.DeleteDetach {
		// Check if no more attached label in the subnet, delete the subnet
		if len(f.database.attachedLabels[fssSubnetID]) == 0 {
			subnet, ok := f.database.subnets[fssSubnetID]
			if ok {
				u := subnetPath + "/" + subnet.ID
				statusCode, _, err := f.DELETE(u)
				if err != nil {
					klog.Errorf("Delete subnet failed with status=%d: %s", statusCode, err.Error())
				}
				klog.Infof("subnet %s is deleted", subnet.ID)
				delete(f.database.subnetMapping[fssWorkloadEvpnID], subnet.FssSubnetName)
				delete(f.database.subnets, fssSubnetID)
				delete(f.database.hostPortLabels, fssSubnetID)
				delete(f.database.attachedLabels, fssSubnetID)
			}
			// Check if no more subnet in the tenant, delete the tenant
			if len(f.database.subnetMapping[fssWorkloadEvpnID]) == 0 {
				tenant, ok := f.database.tenants[fssWorkloadEvpnID]
				if ok {
					u := tenantPath + "/" + tenant.ID
					statusCode, _, err := f.DELETE(u)
					if err != nil {
						klog.Errorf("Delete tenant failed with status=%d: %s", statusCode, err.Error())
					}
					klog.Infof("tenant %s is deleted", tenant.ID)
					delete(f.database.workloadMapping, tenant.FssWorkloadEvpnName)
					delete(f.database.subnetMapping, fssWorkloadEvpnID)
					delete(f.database.tenants, fssWorkloadEvpnID)
				}
			}
		}
	}
	return result
}

// CreateHostPort creates host port
func (f *FssClient) CreateHostPort(node string, port datatypes.JSONNic, isLag bool, parentHostPortID string) (string, error) {
	// Check if port exists
	portName := port["name"].(string)
	hostPortID, ok := f.GetHostPort(node, portName)
	if ok {
		return hostPortID, nil
	}
	klog.Infof("Create hostPort for host %s port %s isLag %t with parentPort %s", node, portName, isLag, parentHostPortID)
	hostPort := HostPort{
		DeploymentID:     f.deployment.ID,
		HostName:         node,
		PortName:         portName,
		IsLag:            isLag,
		MacAddress:       port["mac-address"].(string),
		ParentHostPortID: parentHostPortID,
	}
	jsonRequest, _ := json.Marshal(hostPort)
	statusCode, jsonResponse, err := f.POST(hostPortPath, jsonRequest)
	if err != nil {
		return "", err
	}
	if statusCode != 201 {
		var errorResponse ErrorResponse
		json.Unmarshal(jsonResponse, &errorResponse)
		klog.Errorf("HostPort error: %+v", errorResponse)
		return "", fmt.Errorf("Create hostPort failed with status=%d", statusCode)
	}
	json.Unmarshal(jsonResponse, &hostPort)
	klog.Infof("HostPort is created: %+v", hostPort)
	hostPortID = hostPort.ID
	f.database.hostPorts[node][portName] = hostPortID
	return hostPortID, nil
}

// GetHostPort returns host port if exists
func (f *FssClient) GetHostPort(node string, port string) (string, bool) {
	hostPorts, ok := f.database.hostPorts[node]
	if !ok {
		f.database.hostPorts[node] = make(HostPortIDByName)
		hostPorts = f.database.hostPorts[node]
	}
	// Check if port exists
	hostPortID, ok := hostPorts[port]
	if !ok {
		return "", false
	}
	return hostPortID, true
}

// AttachHostPort attaches host port by host port label
func (f *FssClient) AttachHostPort(hostPortLabelID string, node string, port datatypes.JSONNic) error {
	// Check if port exists
	portName := port["name"].(string)
	hostPortID, ok := f.GetHostPort(node, portName)
	if !ok {
		klog.Errorf("HostPort not exist")
		return fmt.Errorf("HostPort not exist")
	}
	// Check if port is already attached
	for _, v := range f.database.attachedPorts[hostPortLabelID] {
		if _, ok = v[hostPortID]; ok {
			klog.Infof("hostPort %s already attached by association %s", hostPortID, v[hostPortID])
			return nil
		}
	}
	klog.Infof("Add hostPortLabel %s to host %s port %s", hostPortLabelID, node, portName)
	hostPortAssociation := HostPortAssociation{
		DeploymentID:    f.deployment.ID,
		HostPortLabelID: hostPortLabelID,
		HostPortID:      hostPortID,
	}
	jsonRequest, _ := json.Marshal(hostPortAssociation)
	statusCode, jsonResponse, err := f.POST(hostPortAssociationPath, jsonRequest)
	if err != nil {
		return err
	}
	if statusCode != 201 {
		var errorResponse ErrorResponse
		json.Unmarshal(jsonResponse, &errorResponse)
		klog.Errorf("HostPortAssociation error: %+v", errorResponse)
		return fmt.Errorf("Create HostPortAssociation failed with status=%d", statusCode)
	}
	json.Unmarshal(jsonResponse, &hostPortAssociation)
	klog.Infof("HostPortAssociation is created: %+v", hostPortAssociation)
	portAssociation := make(HostPortAssociationIDByPort)
	portAssociation[hostPortID] = hostPortAssociation.ID
	f.database.attachedPorts[hostPortLabelID] = append(f.database.attachedPorts[hostPortLabelID], portAssociation)
	return nil
}

// DetachHostPort detaches host port by host port label
func (f *FssClient) DetachHostPort(hostPortLabelID string, node string, port datatypes.JSONNic) error {
	var result error
	// Check if port exists
	portName := port["name"].(string)
	hostPortID, ok := f.database.hostPorts[node][portName]
	if ok {
		klog.Infof("Remove hostPortLabel %s from host %s port %s", hostPortLabelID, node, portName)
		for k, v := range f.database.attachedPorts[hostPortLabelID] {
			if hostPortAssociationID, ok := v[hostPortID]; ok {
				u := hostPortAssociationPath + "/" + hostPortAssociationID
				statusCode, _, err := f.DELETE(u)
				if err != nil {
					result = err
				}
				if statusCode != 204 {
					result = fmt.Errorf("Delete HostPortAssociation failed with status=%d", statusCode)
				}
				klog.Infof("HostPortAssociation %s is deleted", hostPortAssociationID)
				// Remove locally
				f.database.attachedPorts[hostPortLabelID] = append(f.database.attachedPorts[hostPortLabelID][:k], f.database.attachedPorts[hostPortLabelID][k+1:]...)
			}
		}
	}
	return result
}

// DetachNode delete host port by node
func (f *FssClient) DetachNode(nodeName string) {
	var lagPorts = make(map[string]HostPortIDByName)
	for k, v := range f.database.hostPorts[nodeName] {
		if strings.Contains(k, "bond") {
			_, ok := lagPorts[nodeName]
			if !ok {
				lagPorts[nodeName] = make(HostPortIDByName)
			}
			lagPorts[nodeName][k] = v
		} else {
			u := hostPortPath + "/" + v
			klog.Infof("Delete hostPort %s for host %s port %s", v, nodeName, k)
			status, _, err := f.DELETE(u)
			if err != nil {
				klog.Errorf("Delete hostPort failed with status=%d: %s", status, err.Error())
			}
			if status != 204 {
				klog.Errorf("Delete hostPort failed with status=%d", status)
			}
		}
	}
	// delete lag ports last
	for k, v := range lagPorts[nodeName] {
		u := hostPortPath + "/" + v
		klog.Infof("Delete hostPort %s for host %s port %s", v, nodeName, k)
		status, _, err := f.DELETE(u)
		if err != nil {
			klog.Errorf("Delete hostPort failed with status=%d: %s", status, err.Error())
		}
		if status != 204 {
			klog.Errorf("Delete hostPort failed with status=%d", status)
		}
	}
	// Remove locally
	delete(f.database.hostPorts, nodeName)
}
