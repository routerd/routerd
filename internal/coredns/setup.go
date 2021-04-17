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

package coredns

import (
	"fmt"
	"os"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	dnsv1alpha1 "routerd.net/routerd/apis/dns/v1alpha1"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = dnsv1alpha1.AddToScheme(scheme)

	plugin.Register(pluginName, setup)
}

func setup(c *caddy.Controller) error {
	c.Next()
	if c.NextArg() {
		return plugin.Error(pluginName, c.ArgErr())
	}

	var (
		// Kubernetes Namespace this DHCP Server is deployed in.
		namespace = os.Getenv("KUBERNETES_NAMESPACE")
	)

	if len(namespace) == 0 {
		err := fmt.Errorf(
			"env vars KUBERNETES_NAMESPACE are required")
		exitOnError("invalid configuration:", err)
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		// disable metrics as we are not running any workers.
		MetricsBindAddress: "0",
		Namespace:          namespace,
	})
	exitOnError("creating manager", err)

	// parse selectors
	var zoneSelector labels.Selector
	if zoneSelectorString := os.Getenv("ROUTERD_DNSSERVER_ZONE_SELECTOR"); len(zoneSelectorString) > 0 {
		zoneSelector, err = labels.Parse(zoneSelectorString)
		exitOnError("parsing ROUTERD_DNSSERVER_ZONE_SELECTOR", err)
	}

	var recordSetSelector labels.Selector
	if recordSetSelectorString := os.Getenv("ROUTERD_DNSSERVER_RECORDSET_SELECTOR"); len(recordSetSelectorString) > 0 {
		recordSetSelector, err = labels.Parse(recordSetSelectorString)
		exitOnError("parsing ROUTERD_DNSSERVER_ZONE_SELECTOR", err)
	}

	// create plugin
	routerdPlugin := newRouterdPlugin(
		mgr.GetClient(), ctrl.Log.WithName("dns"), zoneSelector, recordSetSelector)
	if err := routerdPlugin.SetupWithManager(mgr); err != nil {
		exitOnError("unable to create dns controller", err)
	}

	go func() {
		setupLog.Info("starting manager")
		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			exitOnError("problem running manager", err)
		}
	}()

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		routerdPlugin.Next = next
		return routerdPlugin
	})
	return nil
}

func exitOnError(msg string, err error) {
	if err == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "%s: %v", msg, err)
	os.Exit(1)
}
