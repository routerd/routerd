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
	"context"
	"fmt"
	"os"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		ClientDisableCacheFor: []client.Object{
			// Needed so we can query DNSServer on startup,
			// without having to wait for the cache to boot up.
			&dnsv1alpha1.DNSServer{},
		},
	})
	exitOnError("creating manager", err)

	// parse selectors
	var (
		zoneSelector      labels.Selector
		recordSetSelector labels.Selector
	)
	dnsServer := &dnsv1alpha1.DNSServer{}
	if err := mgr.GetClient().Get(
		context.Background(),
		types.NamespacedName{
			Name:      os.Getenv("ROUTERD_DNSSERVER_NAME"),
			Namespace: namespace,
		},
		dnsServer,
	); err != nil {
		exitOnError("getting DNSServer object", err)
	}
	zoneSelector, err = metav1.LabelSelectorAsSelector(&dnsServer.Spec.ZoneSelector)
	if err != nil {
		exitOnError("parsing zoneSelector", err)
	}
	recordSetSelector, err = metav1.LabelSelectorAsSelector(&dnsServer.Spec.RecordSetSelector)
	if err != nil {
		exitOnError("parsing recordSetSelector", err)
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
