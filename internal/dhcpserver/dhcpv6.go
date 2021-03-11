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
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/dhcpv6/server6"
	"github.com/insomniacslk/dhcp/iana"
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

type DHCPv6Server struct {
	log            logr.Logger
	client         client.Client
	namespace      string
	dhcpServerName string
	bindInterface  string

	serverID dhcpv6.Duid
}

func NewDHCPv6Server(
	log logr.Logger, client client.Client, c Config) *DHCPv6Server {
	return &DHCPv6Server{
		log:            log,
		client:         client,
		namespace:      c.Namespace,
		dhcpServerName: c.DHCPServerName,
		bindInterface:  c.BindInterface,
	}
}

func (s *DHCPv6Server) Start(ctx context.Context) error {
	s.log.Info("starting dhcpv6 server")
	inter, err := net.InterfaceByName(s.bindInterface)
	if err != nil {
		return fmt.Errorf("get interface by name: %w", err)
	}

	s.serverID = dhcpv6.Duid{
		Type:          dhcpv6.DUID_LL,
		HwType:        iana.HWTypeEthernet,
		LinkLayerAddr: inter.HardwareAddr,
	}

	server, err := server6.NewServer(s.bindInterface, nil, s.handler)
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

func (s *DHCPv6Server) handler(
	conn net.PacketConn, peer net.Addr, req dhcpv6.DHCPv6) {
	s.log.Info(req.Summary())

	msg, err := req.GetInnerMessage()
	if err != nil {
		s.log.Error(err, "could not get inner message from dhcpv6 request")
		return
	}

	var (
		resp dhcpv6.DHCPv6
	)
	switch msg.Type() {
	case dhcpv6.MessageTypeSolicit:
		if req.GetOneOption(dhcpv6.OptionRapidCommit) != nil {
			resp, err = dhcpv6.NewReplyFromMessage(msg)
		} else {
			resp, err = dhcpv6.NewAdvertiseFromSolicit(msg)
		}

	case dhcpv6.MessageTypeRequest, dhcpv6.MessageTypeConfirm,
		dhcpv6.MessageTypeRenew, dhcpv6.MessageTypeRebind,
		dhcpv6.MessageTypeRelease, dhcpv6.MessageTypeInformationRequest:
		resp, err = dhcpv6.NewReplyFromMessage(msg)

	default:
		s.log.Info("unhandled message type received", "type", msg.Type())
		return
	}
	if err != nil {
		s.log.Error(err, "creating reply from dhcpv6 message")
		return
	}

	if msg.Options.OneIANA() == nil {
		// no IP requested
		s.log.Info("no ip requested")
		if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
			s.log.Error(err, "writing response")
			return
		}
		return
	}

	// Server ID
	if sid := msg.Options.ServerID(); sid != nil {
		// RFC8415 §16.{2,5,7}
		// These message types MUST be discarded if they contain *any* ServerID option
		if msg.MessageType == dhcpv6.MessageTypeSolicit ||
			msg.MessageType == dhcpv6.MessageTypeConfirm ||
			msg.MessageType == dhcpv6.MessageTypeRebind {
			return
		}

		// Approximately all others MUST be discarded if the ServerID doesn't match
		if !sid.Equal(s.serverID) {
			s.log.Info(
				"requested server ID does not match this server's ID",
				"sid", sid, "want", s.serverID)
			return
		}
	} else if msg.MessageType == dhcpv6.MessageTypeRequest ||
		msg.MessageType == dhcpv6.MessageTypeRenew ||
		msg.MessageType == dhcpv6.MessageTypeDecline ||
		msg.MessageType == dhcpv6.MessageTypeRelease {
		// RFC8415 §16.{6,8,10,11}
		// These message types MUST be discarded if they *don't* contain a ServerID option
		return
	}

	// Set Server ID on response
	dhcpv6.WithServerID(s.serverID)(resp)

	ctx := context.Background()
	if err := s.handleRequest(ctx, msg, resp); err != nil {
		s.log.Error(err, "handling request")
		return
	}

	s.log.Info(resp.Summary())
	if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
		s.log.Error(err, "writing response")
		return
	}
}

func (s *DHCPv6Server) handleRequest(
	ctx context.Context, msg *dhcpv6.Message, resp dhcpv6.DHCPv6) error {
	dhcpServer := &dhcpv1alpha1.DHCPServer{}
	if err := s.client.Get(ctx, types.NamespacedName{
		Name:      s.dhcpServerName,
		Namespace: s.namespace,
	}, dhcpServer); err != nil {
		return err
	}
	if dhcpServer.Spec.IPv6 == nil {
		return fmt.Errorf("dhcpv6 was disabled after server start")
	}

	ippool := &ipamv1alpha1.IPv6Pool{}
	if err := s.client.Get(ctx, types.NamespacedName{
		Name:      dhcpServer.Spec.IPv6.Pool.Name,
		Namespace: dhcpServer.Namespace,
	}, ippool); err != nil {
		return err
	}

	lease, err := s.ensureLease(ctx, dhcpServer, msg)
	if err != nil {
		return fmt.Errorf("ensuring IPv6Lease: %w", err)
	}

	if err := s.waitForLease(ctx, s.log, lease); err != nil {
		return fmt.Errorf("waiting on IPv4Lease: %w", err)
	}

	// DNS
	if len(dhcpServer.Spec.IPv6.NameServers) > 0 &&
		msg.IsOptionRequested(dhcpv6.OptionDNSRecursiveNameServer) {
		resp.UpdateOption(dhcpv6.OptDNS(
			stringSliceToIPSlice(dhcpServer.Spec.IPv6.NameServers)...))
	}

	// Client IP
	leaseDuration := 1 * time.Hour
	if ippool.Spec.LeaseDuration != nil {
		leaseDuration = ippool.Spec.LeaseDuration.Duration
	}
	leasedIP, _, err := net.ParseCIDR(lease.Status.Address)
	if err != nil {
		return fmt.Errorf("error parsing leased ip: %w", err)
	}
	resp.AddOption(&dhcpv6.OptIANA{
		IaId: msg.Options.OneIANA().IaId,
		Options: dhcpv6.IdentityOptions{Options: []dhcpv6.Option{
			&dhcpv6.OptIAAddress{
				IPv6Addr:          leasedIP,
				PreferredLifetime: leaseDuration,
				ValidLifetime:     leaseDuration,
			},
		}},
	})

	return nil
}

func (s *DHCPv6Server) ensureLease(
	ctx context.Context, dhcpServer *dhcpv1alpha1.DHCPServer,
	msg *dhcpv6.Message,
) (*ipamv1alpha1.IPv6Lease, error) {
	duid := msg.Options.ClientID()
	if duid == nil {
		return nil, fmt.Errorf("can't get Client DUID")
	}

	leaseName := strings.ToLower(
		base32.StdEncoding.EncodeToString(duid.ToBytes()))
	leaseName = strings.Trim(leaseName, "=")

	lease := &ipamv1alpha1.IPv6Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dhcpServer.Name + "-" + leaseName,
			Namespace: s.namespace,
			Labels: map[string]string{
				dhcpDUIDLabel: leaseName,
			},
		},
		Spec: ipamv1alpha1.IPv6LeaseSpec{
			Type: ipamv1alpha1.IPLeaseTypeDynamic,
			Pool: ipamv1alpha1.LocalObjectReference{
				Name: dhcpServer.Spec.IPv6.Pool.Name,
			},
		},
	}

	fqdnOpt := msg.Options.FQDN()
	if fqdnOpt != nil {
		fqdn := strings.Join(fqdnOpt.DomainName.Labels, ".")
		host := fqdn
		if i := strings.Index(host, "."); i != -1 {
			host = host[:i]
		}

		lease.Labels[dhcpFQDNLabel] = fqdn
		lease.Labels[dhcpHostLabel] = host
	}
	if err := controllerutil.SetOwnerReference(
		dhcpServer, lease, s.client.Scheme()); err != nil {
		return nil, err
	}

	currentLease, err := reconcile.IPv6Lease(ctx, s.client, lease)
	if err != nil {
		return nil, err
	}

	// renew lease if exists
	currentLease.Spec.RenewTime = metav1.NowMicro()
	return currentLease,
		retryOnConflict(func() error { return s.client.Update(ctx, currentLease) })
}

func (s *DHCPv6Server) waitForLease(
	ctx context.Context, log logr.Logger, lease *ipamv1alpha1.IPv6Lease,
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
			log.Info("done waiting on", "ipv6Lease", nn.String())
			return true, nil
		}
		return false, nil
	}, ctx.Done())
}
