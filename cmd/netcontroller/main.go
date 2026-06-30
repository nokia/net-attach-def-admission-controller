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

// This is NCS VLAN Operator.
package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	coreSharedInformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"

	clientset "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned"
	sharedInformers "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/informers/externalversions"

	"github.com/nokia/net-attach-def-admission-controller/pkg/netcontroller"
)

var (
	// defines default resync period between k8s API server and controller
	syncPeriod = time.Second * 600
)

func main() {
	var (
		provider string
		nodeName = os.Getenv("NODE_NAME")
	)

	klog.InitFlags(nil)
	flag.StringVar(&provider, "provider", "baremetal", "Only baremetal and openstack are supported.")
	flag.Parse()

	cfg, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf("error building kubeconfig: %s", err.Error())
	}

	k8sClientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("error creating kubernetes clientset: %s", err.Error())
	}

	netAttachDefClientSet, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("error creating net-attach-def clientset: %s", err.Error())
	}

	netAttachDefInformerFactory := sharedInformers.NewSharedInformerFactory(netAttachDefClientSet, syncPeriod)
	k8sInformerFactory := coreSharedInformers.NewSharedInformerFactory(k8sClientSet, syncPeriod)

	networkController := netcontroller.NewNetworkController(
		provider,
		nodeName,
		k8sClientSet,
		netAttachDefClientSet,
		netAttachDefInformerFactory.K8sCniCncfIo().V1().NetworkAttachmentDefinitions(),
		k8sInformerFactory.Core().V1().Nodes(),
	)

	stopChan := make(chan struct{})
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	go func() {
		<-c
		close(stopChan)
		<-c
		os.Exit(1)
	}()

	netAttachDefInformerFactory.Start(stopChan)
	k8sInformerFactory.Start(stopChan)
	networkController.Start(stopChan)
}
