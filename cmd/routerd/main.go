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

package main

import (
	"context"
	"flag"
	"net/http"
	"net/http/pprof"
	"os"

	netv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/metal-stack/go-ipam"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	dhcpv1alpha1 "routerd.net/routerd/apis/dhcp/v1alpha1"
	dnsv1alpha1 "routerd.net/routerd/apis/dns/v1alpha1"
	ipamv1alpha1 "routerd.net/routerd/apis/ipam/v1alpha1"
	dhcpcontrollers "routerd.net/routerd/internal/controllers/dhcp"
	dnscontrollers "routerd.net/routerd/internal/controllers/dns"
	ipamcontrollers "routerd.net/routerd/internal/controllers/ipam"
	ipamadapter "routerd.net/routerd/internal/controllers/ipam/adapter"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = ipamv1alpha1.AddToScheme(scheme)
	_ = dhcpv1alpha1.AddToScheme(scheme)
	_ = dnsv1alpha1.AddToScheme(scheme)
	_ = netv1.AddToScheme(scheme)
}

func main() {
	var (
		metricsAddr          string
		pprofAddr            string
		enableLeaderElection bool
	)
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&pprofAddr, "pprof-addr", "", "The address the pprof web endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                     scheme,
		MetricsBindAddress:         metricsAddr,
		Port:                       9443,
		LeaderElectionResourceLock: "leases",
		LeaderElection:             enableLeaderElection,
		LeaderElectionID:           "32bf6c51.routerd.net",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// -----
	// PPROF
	// -----
	if len(pprofAddr) > 0 {
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

		s := &http.Server{Addr: pprofAddr, Handler: mux}
		err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
			errCh := make(chan error)
			defer func() {
				for range errCh {
				} // drain errCh for GC
			}()
			go func() {
				defer close(errCh)
				errCh <- s.ListenAndServe()
			}()

			select {
			case err := <-errCh:
				return err
			case <-ctx.Done():
				s.Close()
				return nil
			}
		}))
		if err != nil {
			setupLog.Error(err, "unable to create pprof server")
			os.Exit(1)
		}
	}

	// ----
	// DHCP
	// ----
	if err = (&dhcpcontrollers.DHCPServerReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("DHCPServer"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DHCPServer")
		os.Exit(1)
	}

	// ----
	// DNS
	// ----
	if err = (&dnscontrollers.DNSServerReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("DNSServer"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DNSServer")
		os.Exit(1)
	}

	// ----
	// IPAM
	// ----

	ipamCache := ipamcontrollers.NewIPAMCache()
	var (
		ipv4PoolType      = ipamadapter.AdaptIPPool(&ipamv1alpha1.IPv4Pool{})
		ipv4LeaseType     = ipamadapter.AdaptIPLease(&ipamv1alpha1.IPv4Lease{})
		ipv4LeaseListType = ipamadapter.AdaptIPLeaseList(&ipamv1alpha1.IPv4LeaseList{})

		ipv6PoolType      = ipamadapter.AdaptIPPool(&ipamv1alpha1.IPv6Pool{})
		ipv6LeaseType     = ipamadapter.AdaptIPLease(&ipamv1alpha1.IPv6Lease{})
		ipv6LeaseListType = ipamadapter.AdaptIPLeaseList(&ipamv1alpha1.IPv6LeaseList{})
	)

	// IPv4 Controllers
	if err = (&ipamcontrollers.IPPoolReconciler{
		Client:    mgr.GetClient(),
		Log:       ctrl.Log.WithName("controllers").WithName("IPv4Pool"),
		Scheme:    mgr.GetScheme(),
		IPAMCache: ipamCache,
		NewIPAM:   func() ipamcontrollers.Ipamer { return ipam.New() },

		IPPoolType:      ipv4PoolType,
		IPLeaseType:     ipv4LeaseType,
		IPLeaseListType: ipv4LeaseListType,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IPv4Pool")
		os.Exit(1)
	}
	if err = (&ipamcontrollers.IPLeaseReconciler{
		Client:    mgr.GetClient(),
		Log:       ctrl.Log.WithName("controllers").WithName("IPv4Lease"),
		Scheme:    mgr.GetScheme(),
		IPAMCache: ipamCache,

		IPPoolType:  ipv4PoolType,
		IPLeaseType: ipv4LeaseType,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IPv4Lease")
		os.Exit(1)
	}

	// IPv6 Controllers
	if err = (&ipamcontrollers.IPPoolReconciler{
		Client:    mgr.GetClient(),
		Log:       ctrl.Log.WithName("controllers").WithName("IPv6Pool"),
		Scheme:    mgr.GetScheme(),
		IPAMCache: ipamCache,
		NewIPAM:   func() ipamcontrollers.Ipamer { return ipam.New() },

		IPPoolType:      ipv6PoolType,
		IPLeaseType:     ipv6LeaseType,
		IPLeaseListType: ipv6LeaseListType,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IPv6Pool")
		os.Exit(1)
	}
	if err = (&ipamcontrollers.IPLeaseReconciler{
		Client:    mgr.GetClient(),
		Log:       ctrl.Log.WithName("controllers").WithName("IPv6Lease"),
		Scheme:    mgr.GetScheme(),
		IPAMCache: ipamCache,

		IPPoolType:  ipv6PoolType,
		IPLeaseType: ipv6LeaseType,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IPv6Lease")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
