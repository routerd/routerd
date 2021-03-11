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

package dhcpserver

import (
	"context"
	"encoding/base32"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dhcpv1alpha1 "routerd.net/routerd/apis/dhcp/v1alpha1"
	ipamv1alpha1 "routerd.net/routerd/apis/ipam/v1alpha1"
	"routerd.net/routerd/internal/reconcile"
)

type DHCPv4Server struct {
	log            logr.Logger
	client         client.Client
	namespace      string
	dhcpServerName string
	bindInterface  string
}

func NewDHCPv4Server(
	log logr.Logger, client client.Client, c Config) *DHCPv4Server {
	return &DHCPv4Server{
		log:            log,
		client:         client,
		namespace:      c.Namespace,
		dhcpServerName: c.DHCPServerName,
		bindInterface:  c.BindInterface,
	}
}

func (s *DHCPv4Server) Start(ctx context.Context) error {
	s.log.Info("starting dhcpv4 server")
	server, err := server4.NewServer(s.bindInterface, nil, s.handler)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	doneCh := make(chan error)
	go func() {
		defer close(doneCh)
		doneCh <- server.Serve()
	}()

	select {
	case err := <-doneCh:
		return err
	case <-ctx.Done():
		server.Close()
		<-doneCh
		return nil
	}
}

func (s *DHCPv4Server) handler(
	conn net.PacketConn, peer net.Addr, req *dhcpv4.DHCPv4) {
	s.log.Info(req.Summary())

	if req.OpCode != dhcpv4.OpcodeBootRequest {
		// only support BootRequests
		return
	}

	resp, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		s.log.Error(err, "could not create reply for request")
		return
	}

	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	case dhcpv4.MessageTypeRequest:
		resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	}

	ctx := context.Background()
	if err := s.handleRequest(ctx, req, resp); err != nil {
		s.log.Error(err, "handling request")
		return
	}

	s.log.Info(resp.Summary())
	if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
		s.log.Error(err, "writing response")
		return
	}
}

func (s *DHCPv4Server) handleRequest(
	ctx context.Context, req *dhcpv4.DHCPv4, resp *dhcpv4.DHCPv4) error {
	dhcpServer := &dhcpv1alpha1.DHCPServer{}
	if err := s.client.Get(ctx, types.NamespacedName{
		Name:      s.dhcpServerName,
		Namespace: s.namespace,
	}, dhcpServer); err != nil {
		return err
	}
	if dhcpServer.Spec.IPv4 == nil {
		return fmt.Errorf("dhcpv4 was disabled after server start")
	}

	ippool := &ipamv1alpha1.IPv4Pool{}
	if err := s.client.Get(ctx, types.NamespacedName{
		Name:      dhcpServer.Spec.IPv4.Pool.Name,
		Namespace: dhcpServer.Namespace,
	}, ippool); err != nil {
		return err
	}

	lease, err := s.ensureLease(ctx, dhcpServer, req)
	if err != nil {
		return fmt.Errorf("ensuring IPv4Lease: %w", err)
	}

	if err := s.waitForLease(ctx, s.log, lease); err != nil {
		return fmt.Errorf("waiting on IPv4Lease: %w", err)
	}

	// Subnet Mask
	_, ipv4net, _ := net.ParseCIDR(ippool.Spec.CIDR)
	resp.Options.Update(dhcpv4.OptSubnetMask(ipv4net.Mask))

	// Lease Time
	leaseDuration := 1 * time.Hour
	if ippool.Spec.LeaseDuration != nil {
		leaseDuration = ippool.Spec.LeaseDuration.Duration
	}
	resp.Options.Update(
		dhcpv4.OptIPAddressLeaseTime(leaseDuration))

	// Server ID
	serverIP, _, err := net.ParseCIDR(dhcpServer.Status.IPv4Address)
	if err != nil {
		return fmt.Errorf("error parsing server ip: %w", err)
	}
	resp.UpdateOption(
		dhcpv4.OptServerIdentifier(serverIP))

	// Router
	resp.Options.Update(
		dhcpv4.OptRouter(net.ParseIP(dhcpServer.Spec.IPv4.Gateway)))

	// DNS
	if len(dhcpServer.Spec.IPv4.NameServers) > 0 &&
		req.IsOptionRequested(dhcpv4.OptionDomainNameServer) {
		resp.Options.Update(dhcpv4.OptDNS(
			stringSliceToIPSlice(dhcpServer.Spec.IPv4.NameServers)...))
	}

	// Leased IP
	yourIP, _, err := net.ParseCIDR(lease.Status.Address)
	if err != nil {
		return fmt.Errorf("error parsing leased ip: %w", err)
	}
	resp.YourIPAddr = yourIP
	return nil
}

func (s *DHCPv4Server) ensureLease(
	ctx context.Context, dhcpServer *dhcpv1alpha1.DHCPServer, req *dhcpv4.DHCPv4,
) (*ipamv1alpha1.IPv4Lease, error) {
	leaseName := strings.ToLower(
		base32.StdEncoding.EncodeToString(req.ClientHWAddr))
	leaseName = strings.Trim(leaseName, "=")

	lease := &ipamv1alpha1.IPv4Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dhcpServer.Name + "-" + leaseName,
			Namespace: s.namespace,
			Labels: map[string]string{
				dhcpMACLabel: leaseName,
			},
		},
		Spec: ipamv1alpha1.IPv4LeaseSpec{
			Type: ipamv1alpha1.IPLeaseTypeDynamic,
			Pool: ipamv1alpha1.LocalObjectReference{
				Name: dhcpServer.Spec.IPv4.Pool.Name,
			},
		},
	}
	hostbytes := req.Options.Get(dhcpv4.OptionHostName)
	if len(hostbytes) > 0 {
		lease.Labels[dhcpHostLabel] = string(hostbytes)
	}
	if err := controllerutil.SetOwnerReference(
		dhcpServer, lease, s.client.Scheme()); err != nil {
		return nil, err
	}

	currentLease, err := reconcile.IPv4Lease(ctx, s.client, lease)
	if err != nil {
		return nil, err
	}

	// renew lease if exists
	currentLease.Spec.RenewTime = metav1.NowMicro()
	return currentLease,
		retryOnConflict(func() error { return s.client.Update(ctx, currentLease) })
}

func (s *DHCPv4Server) waitForLease(
	ctx context.Context, log logr.Logger, lease *ipamv1alpha1.IPv4Lease,
) error {
	ctx, cancel := context.WithTimeout(ctx, leaseAcquireTimeout)
	defer cancel()

	nn := types.NamespacedName{
		Name:      lease.GetName(),
		Namespace: lease.GetNamespace(),
	}

	return wait.PollImmediateUntil(leasePollInterval, func() (bool, error) {
		err := s.client.Get(ctx, nn, lease)
		if errors.IsNotFound(err) {
			// the object is probably just not yet pushed into our cache.
			return false, nil
		}
		if err != nil {
			log.Error(err, "error waiting")
			// treat as a transient error and hope it will go away.
			return false, nil
		}

		if meta.IsStatusConditionTrue(
			lease.Status.Conditions, ipamv1alpha1.IPLeaseBound) {
			// yay!
			log.Info("done waiting on", "ipv4Lease", nn.String())
			return true, nil
		}
		return false, nil
	}, ctx.Done())
}
