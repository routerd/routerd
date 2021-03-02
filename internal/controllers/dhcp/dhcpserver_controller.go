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

package dhcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/go-logr/logr"
	netv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dhcpv1alpha1 "routerd.net/routerd/apis/dhcp/v1alpha1"
	ipamv1alpha1 "routerd.net/routerd/apis/ipam/v1alpha1"
	"routerd.net/routerd/internal/reconcile"
)

// DHCPServerReconciler reconciles a DHCPServer object
type DHCPServerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func (r *DHCPServerReconciler) Reconcile(
	ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	dhcpServer := &dhcpv1alpha1.DHCPServer{}
	if err = r.Get(ctx, req.NamespacedName, dhcpServer); err != nil {
		return res, client.IgnoreNotFound(err)
	}

	var (
		ipv4Enabled = dhcpServer.Spec.IPv4 != nil
		ipv6Enabled = dhcpServer.Spec.IPv6 != nil
	)
	if !ipv4Enabled && !ipv6Enabled {
		return res, nil
	}

	// Phase 1.
	// Lookup IPPools
	var (
		ipv4Pool      *ipamv1alpha1.IPv4Pool
		ipv6Pool      *ipamv1alpha1.IPv6Pool
		poolsNotFound []string
	)
	if ipv4Enabled {
		ippool := &ipamv1alpha1.IPv4Pool{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      dhcpServer.Spec.IPv4.Pool.Name,
			Namespace: dhcpServer.Namespace,
		}, ippool)
		switch {
		case err == nil:
			ipv4Pool = ippool

		case errors.IsNotFound(err):
			poolsNotFound = append(
				poolsNotFound, "IPv4 "+dhcpServer.Spec.IPv4.Pool.Name)

		default:
			return res, err
		}
	}
	if ipv6Enabled {
		ippool := &ipamv1alpha1.IPv6Pool{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      dhcpServer.Spec.IPv6.Pool.Name,
			Namespace: dhcpServer.Namespace,
		}, ippool)
		switch {
		case err == nil:
			ipv6Pool = ippool

		case errors.IsNotFound(err):
			poolsNotFound = append(
				poolsNotFound, "IPv6 "+dhcpServer.Spec.IPv6.Pool.Name)

		default:
			return res, err
		}
	}
	if len(poolsNotFound) > 0 {
		dhcpServer.Status.Phase = "IPPoolNotFound"
		dhcpServer.Status.ObservedGeneration = dhcpServer.Generation
		meta.SetStatusCondition(&dhcpServer.Status.Conditions, metav1.Condition{
			Type:   dhcpv1alpha1.Available,
			Status: metav1.ConditionFalse,
			Reason: "IPPoolNotFound",
			Message: fmt.Sprintf(
				"IPPools %s not found", strings.Join(poolsNotFound, ", ")),
			ObservedGeneration: dhcpServer.Generation,
		})
		res.Requeue = true // check later
		return res, r.Status().Update(ctx, dhcpServer)
	}

	// Phase 2.
	// Ensure the Gateway IP Address has a Static Lease,
	// so it's not allocated by the DHCP Server.
	ipv4GatewayLease, err := r.ensureIPv4GatewayLease(ctx, dhcpServer, ipv4Pool)
	if err != nil {
		return res, fmt.Errorf("ensuring IPv4Lease for Gateway IP: %w", err)
	}
	ipv6GatewayLease, err := r.ensureIPv6GatewayLease(ctx, dhcpServer, ipv6Pool)
	if err != nil {
		return res, fmt.Errorf("ensuring IPv6Lease for Gateway IP: %w", err)
	}

	// check status and wait
	var (
		unreadyIPLease []string
		failedIPLease  []string
	)
	if ipv4GatewayLease != nil {
		switch {
		case meta.IsStatusConditionTrue(
			ipv4GatewayLease.Status.Conditions, ipamv1alpha1.IPLeaseBound):
			// ready!

		case meta.IsStatusConditionFalse(
			ipv4GatewayLease.Status.Conditions, ipamv1alpha1.IPLeaseBound):
			failedIPLease = append(failedIPLease, ipv4GatewayLease.Status.Address)

		default:
			unreadyIPLease = append(unreadyIPLease, ipv4GatewayLease.Status.Address)
		}
	}
	if ipv6GatewayLease != nil {
		switch {
		case meta.IsStatusConditionTrue(
			ipv6GatewayLease.Status.Conditions, ipamv1alpha1.IPLeaseBound):
			// ready!

		case meta.IsStatusConditionFalse(
			ipv6GatewayLease.Status.Conditions, ipamv1alpha1.IPLeaseBound):
			failedIPLease = append(failedIPLease, ipv6GatewayLease.Status.Address)

		default:
			unreadyIPLease = append(unreadyIPLease, ipv6GatewayLease.Status.Address)
		}
	}
	if len(failedIPLease) > 0 {
		// Failed Leasing GW Addresses
		meta.SetStatusCondition(&dhcpServer.Status.Conditions, metav1.Condition{
			Type:   dhcpv1alpha1.Available,
			Status: metav1.ConditionFalse,
			Reason: "UnboundGatewayIP",
			Message: fmt.Sprintf(
				"Could not lease Gateway IPs: %s",
				strings.Join(failedIPLease, ", ")),
			ObservedGeneration: dhcpServer.Generation,
		})
		dhcpServer.Status.ObservedGeneration = dhcpServer.Generation
		dhcpServer.Status.Phase = "Failed"
		return res, r.Status().Update(ctx, dhcpServer)
	}
	if len(unreadyIPLease) > 0 {
		// Unready GW Addresses, waiting
		meta.SetStatusCondition(&dhcpServer.Status.Conditions, metav1.Condition{
			Type:   dhcpv1alpha1.Available,
			Status: metav1.ConditionFalse,
			Reason: "UnboundGatewayIP",
			Message: fmt.Sprintf(
				"Pending lease on Gateway IPs: %s",
				strings.Join(unreadyIPLease, ", ")),
			ObservedGeneration: dhcpServer.Generation,
		})
		dhcpServer.Status.ObservedGeneration = dhcpServer.Generation
		dhcpServer.Status.Phase = "Pending"
		return res, r.Status().Update(ctx, dhcpServer)
	}

	// Phase 3.
	// Lease IP addresses for the DHCP Server
	var (
		unreadyDHCPIPLease []string
		failedDHCPIPLease  []string
	)
	ipv4DHCPLease, err := r.ensureIPv4Lease(ctx, dhcpServer, ipv4Pool)
	if err != nil {
		return res, fmt.Errorf("ensuring IPv4Lease for DHCP server: %w", err)
	}
	ipv6DHCPLease, err := r.ensureIPv6Lease(ctx, dhcpServer, ipv6Pool)
	if err != nil {
		return res, fmt.Errorf("ensuring IPv6Lease for DHCP server: %w", err)
	}
	if ipv4DHCPLease != nil {
		switch {
		case meta.IsStatusConditionTrue(
			ipv4GatewayLease.Status.Conditions, ipamv1alpha1.IPLeaseBound):
			dhcpServer.Status.IPv4Address = ipv4DHCPLease.Status.Address

		case meta.IsStatusConditionFalse(
			ipv4GatewayLease.Status.Conditions, ipamv1alpha1.IPLeaseBound):
			failedDHCPIPLease = append(failedDHCPIPLease, ipv4GatewayLease.Name)

		default:
			unreadyDHCPIPLease = append(unreadyDHCPIPLease, ipv4GatewayLease.Name)
		}
	}
	if ipv6DHCPLease != nil {
		switch {
		case meta.IsStatusConditionTrue(
			ipv6GatewayLease.Status.Conditions, ipamv1alpha1.IPLeaseBound):
			dhcpServer.Status.IPv6Address = ipv6DHCPLease.Status.Address

		case meta.IsStatusConditionFalse(
			ipv6GatewayLease.Status.Conditions, ipamv1alpha1.IPLeaseBound):
			failedDHCPIPLease = append(failedDHCPIPLease, ipv6GatewayLease.Name)

		default:
			unreadyDHCPIPLease = append(unreadyDHCPIPLease, ipv6GatewayLease.Name)
		}
	}
	if len(failedDHCPIPLease) > 0 {
		// Failed Leasing GW Addresses
		meta.SetStatusCondition(&dhcpServer.Status.Conditions, metav1.Condition{
			Type:   dhcpv1alpha1.Available,
			Status: metav1.ConditionFalse,
			Reason: "UnboundDHCPServerIP",
			Message: fmt.Sprintf(
				"Could not lease IPs for DHCP server: %s",
				strings.Join(failedDHCPIPLease, ", ")),
			ObservedGeneration: dhcpServer.Generation,
		})
		dhcpServer.Status.ObservedGeneration = dhcpServer.Generation
		dhcpServer.Status.Phase = "Failed"
		return res, r.Status().Update(ctx, dhcpServer)
	}
	if len(unreadyDHCPIPLease) > 0 {
		// Unready GW Addresses, waiting
		meta.SetStatusCondition(&dhcpServer.Status.Conditions, metav1.Condition{
			Type:   dhcpv1alpha1.Available,
			Status: metav1.ConditionFalse,
			Reason: "UnboundDHCPServerIP",
			Message: fmt.Sprintf(
				"Pending IPLease on DHCP server: %s",
				strings.Join(unreadyDHCPIPLease, ", ")),
			ObservedGeneration: dhcpServer.Generation,
		})
		dhcpServer.Status.ObservedGeneration = dhcpServer.Generation
		dhcpServer.Status.Phase = "Pending"
		return res, r.Status().Update(ctx, dhcpServer)
	}

	// Phase 4.
	// Create NAD and Deployment

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dhcpServer.Name + "dhcp-server",
			Namespace: dhcpServer.Namespace,
		},
	}
	_, err = reconcile.ServiceAccount(ctx, r.Client, sa)
	if err != nil {
		return res, fmt.Errorf("reconcile ServiceAccount: %w", err)
	}

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sa.Name,
			Namespace: sa.Namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: sa.Name,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "routerd-dhcp-role",
		},
	}
	_, err = reconcile.RoleBinding(ctx, r.Client, roleBinding)
	if err != nil {
		return res, fmt.Errorf("reconcile RoleBinding: %w", err)
	}

	// NetworkAttachmentDefinition
	nad, err := r.ensureNetworkAttachmentDefinition(
		ctx, dhcpServer, ipv4DHCPLease, ipv6DHCPLease, ipv4Pool, ipv6Pool)
	if err != nil {
		return res, err
	}

	deploy, stop, err := r.ensureDeployment(ctx, dhcpServer, ipv6Pool, nad, sa)
	if err != nil {
		return res, err
	} else if stop {
		return res, nil
	}

	dhcpServer.Status.ObservedGeneration = dhcpServer.Generation
	if deploy.Status.AvailableReplicas == deploy.Status.Replicas {
		dhcpServer.Status.Phase = "Ready"
		meta.SetStatusCondition(&dhcpServer.Status.Conditions, metav1.Condition{
			Type:               dhcpv1alpha1.Available,
			Status:             metav1.ConditionTrue,
			Reason:             "DeploymentReady",
			Message:            "DHCP Server Deployment ready",
			ObservedGeneration: deploy.Generation,
		})
	} else {
		dhcpServer.Status.Phase = "Unready"
		meta.SetStatusCondition(&dhcpServer.Status.Conditions, metav1.Condition{
			Type:               dhcpv1alpha1.Available,
			Status:             metav1.ConditionFalse,
			Reason:             "DeploymentUnready",
			Message:            "DHCP Server Deployment is not ready",
			ObservedGeneration: deploy.Generation,
		})
	}
	if err = r.Status().Update(ctx, dhcpServer); err != nil {
		return
	}
	return
}

func (r *DHCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dhcpv1alpha1.DHCPServer{}).
		Owns(&ipamv1alpha1.IPv4Lease{}).
		Owns(&ipamv1alpha1.IPv6Lease{}).
		Owns(&netv1.NetworkAttachmentDefinition{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

// Ensure a Lease for the IPv4 Gateway exists.
func (r *DHCPServerReconciler) ensureIPv4GatewayLease(
	ctx context.Context, dhcpServer *dhcpv1alpha1.DHCPServer,
	ippool *ipamv1alpha1.IPv4Pool,
) (_ *ipamv1alpha1.IPv4Lease, err error) {
	if dhcpServer.Spec.IPv4 == nil {
		return nil, nil
	}

	// Ensure Gateway IP is Leased
	gwlease := &ipamv1alpha1.IPv4Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ippool.Name + "-gateway",
			Namespace: ippool.Namespace,
			Labels:    map[string]string{},
		},
		Spec: ipamv1alpha1.IPv4LeaseSpec{
			Pool: ipamv1alpha1.LocalObjectReference{
				Name: ippool.Name,
			},
			Type: ipamv1alpha1.IPLeaseTypeStatic,
			Static: &ipamv1alpha1.IPLeaseStatic{
				Address: dhcpServer.Spec.IPv4.Gateway,
			},
		},
	}
	addCommonLabels(gwlease.Labels, dhcpServer)
	if err := controllerutil.SetControllerReference(
		dhcpServer, gwlease, r.Scheme); err != nil {
		return nil, err
	}

	currentGWLease, err := reconcile.IPv4Lease(ctx, r.Client, gwlease)
	if err != nil {
		return nil, err
	}
	return currentGWLease, nil
}

// Ensure a Lease for the IPv6 Gateway exists.
func (r *DHCPServerReconciler) ensureIPv6GatewayLease(
	ctx context.Context, dhcpServer *dhcpv1alpha1.DHCPServer,
	ippool *ipamv1alpha1.IPv6Pool,
) (_ *ipamv1alpha1.IPv6Lease, err error) {
	if dhcpServer.Spec.IPv6 == nil {
		return nil, nil
	}

	// Ensure Gateway IP is Leased
	gwlease := &ipamv1alpha1.IPv6Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ippool.Name + "-gateway",
			Namespace: ippool.Namespace,
			Labels:    map[string]string{},
		},
		Spec: ipamv1alpha1.IPv6LeaseSpec{
			Pool: ipamv1alpha1.LocalObjectReference{
				Name: ippool.Name,
			},
			Type: ipamv1alpha1.IPLeaseTypeStatic,
			Static: &ipamv1alpha1.IPLeaseStatic{
				Address: dhcpServer.Spec.IPv6.Gateway,
			},
		},
	}
	addCommonLabels(gwlease.Labels, dhcpServer)
	if err := controllerutil.SetControllerReference(
		dhcpServer, gwlease, r.Scheme); err != nil {
		return nil, err
	}

	currentGWLease, err := reconcile.IPv6Lease(ctx, r.Client, gwlease)
	if err != nil {
		return nil, err
	}
	return currentGWLease, nil
}

// ensures the DHCP Server Pod has a Lease for it's own IP address.
func (r *DHCPServerReconciler) ensureIPv4Lease(
	ctx context.Context,
	dhcpServer *dhcpv1alpha1.DHCPServer,
	ippool *ipamv1alpha1.IPv4Pool,
) (_ *ipamv1alpha1.IPv4Lease, err error) {
	if dhcpServer.Spec.IPv4 == nil {
		return nil, nil
	}

	iplease := &ipamv1alpha1.IPv4Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ippool.Name + "-dhcp",
			Namespace: ippool.Namespace,
			Labels:    map[string]string{},
		},
		Spec: ipamv1alpha1.IPv4LeaseSpec{
			Pool: ipamv1alpha1.LocalObjectReference{
				Name: ippool.Name,
			},
			Type: ipamv1alpha1.IPLeaseTypeStatic,
		},
	}
	addCommonLabels(iplease.Labels, dhcpServer)
	if err := controllerutil.SetControllerReference(dhcpServer, iplease, r.Scheme); err != nil {
		return nil, err
	}

	currentIPLease, err := reconcile.IPv4Lease(ctx, r.Client, iplease)
	if err != nil {
		return nil, fmt.Errorf("reconciling gateway IPLease: %w", err)
	}
	return currentIPLease, nil
}

// ensures the DHCP Server Pod has a Lease for it's own IP address.
func (r *DHCPServerReconciler) ensureIPv6Lease(
	ctx context.Context,
	dhcpServer *dhcpv1alpha1.DHCPServer,
	ippool *ipamv1alpha1.IPv6Pool,
) (_ *ipamv1alpha1.IPv6Lease, err error) {
	if dhcpServer.Spec.IPv6 == nil {
		return nil, nil
	}

	iplease := &ipamv1alpha1.IPv6Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ippool.Name + "-dhcp",
			Namespace: ippool.Namespace,
			Labels:    map[string]string{},
		},
		Spec: ipamv1alpha1.IPv6LeaseSpec{
			Pool: ipamv1alpha1.LocalObjectReference{
				Name: ippool.Name,
			},
			Type: ipamv1alpha1.IPLeaseTypeStatic,
		},
	}
	addCommonLabels(iplease.Labels, dhcpServer)
	if err := controllerutil.SetControllerReference(dhcpServer, iplease, r.Scheme); err != nil {
		return nil, err
	}

	currentIPLease, err := reconcile.IPv6Lease(ctx, r.Client, iplease)
	if err != nil {
		return nil, fmt.Errorf("reconciling gateway IPLease: %w", err)
	}
	return currentIPLease, nil
}

func (r *DHCPServerReconciler) ensureNetworkAttachmentDefinition(
	ctx context.Context,
	dhcpServer *dhcpv1alpha1.DHCPServer,
	dhcpIPv4Lease *ipamv1alpha1.IPv4Lease,
	dhcpIPv6Lease *ipamv1alpha1.IPv6Lease,
	ipv4Pool *ipamv1alpha1.IPv4Pool,
	ipv6Pool *ipamv1alpha1.IPv6Pool,
) (_ *netv1.NetworkAttachmentDefinition, err error) {
	if dhcpServer.Spec.NetworkAttachment.Type != dhcpv1alpha1.Bridge {
		return nil, fmt.Errorf(
			"unsupported network attachment type: %s",
			dhcpServer.Spec.NetworkAttachment.Type)
	}

	var (
		ipv4Mask net.IPMask
		ipv6Mask net.IPMask
	)
	var addresses []map[string]string
	if dhcpServer.Spec.IPv4 != nil {
		_, ipv4Net, _ := net.ParseCIDR(ipv4Pool.Spec.CIDR)
		ipv4Mask = ipv4Net.Mask
		ip := &net.IPNet{
			IP:   net.ParseIP(dhcpIPv4Lease.Status.Address),
			Mask: ipv4Mask,
		}
		addresses = append(addresses, map[string]string{
			"address": ip.String()})
	}
	if dhcpServer.Spec.IPv6 != nil {
		_, ipv6Net, _ := net.ParseCIDR(ipv6Pool.Spec.CIDR)
		ipv6Mask = ipv6Net.Mask
		ip := &net.IPNet{
			IP:   net.ParseIP(dhcpIPv6Lease.Status.Address),
			Mask: ipv6Mask,
		}
		addresses = append(addresses, map[string]string{
			"address": ip.String()})
	}

	nadConfigJSON := map[string]interface{}{
		"cniVersion": "0.3.1",
		"name":       dhcpServer.Namespace + "-" + dhcpServer.Name,
		"plugins": []map[string]interface{}{
			{
				"type":   "bridge",
				"bridge": dhcpServer.Spec.NetworkAttachment.Bridge.Name,
				"ipam": map[string]interface{}{
					"type":      "static",
					"addresses": addresses,
				},
			},
		},
	}
	nadConfigBytes, err := json.MarshalIndent(nadConfigJSON, "", "  ")
	if err != nil {
		return nil, err
	}

	nad := &netv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dhcpServer.Name,
			Namespace: dhcpServer.Namespace,
			Labels:    map[string]string{},
		},
		Spec: netv1.NetworkAttachmentDefinitionSpec{
			Config: string(nadConfigBytes),
		},
	}
	addCommonLabels(nad.Labels, dhcpServer)
	if err := controllerutil.SetControllerReference(dhcpServer, nad, r.Scheme); err != nil {
		return nil, err
	}

	if _, err := reconcileNAD(ctx, r.Client, nad); err != nil {
		return nil, fmt.Errorf("reconciling NetworkAttachmentDefinition: %w", err)
	}
	return nad, nil
}

func (r *DHCPServerReconciler) ensureDeployment(
	ctx context.Context,
	dhcpServer *dhcpv1alpha1.DHCPServer,
	ipv6pool *ipamv1alpha1.IPv6Pool,
	nad *netv1.NetworkAttachmentDefinition,
	sa *corev1.ServiceAccount,
) (_ *appsv1.Deployment, stop bool, err error) {
	deploy, err := deployment(r.Scheme, dhcpServer, ipv6pool, nad, sa)
	if err != nil {
		return nil, false, fmt.Errorf("preparing Deployment: %w", err)
	}
	if _, err := reconcileDeployment(ctx, r.Client, deploy); err != nil {
		return nil, false, fmt.Errorf("reconciling Deployment: %w", err)
	}
	return deploy, false, nil
}
