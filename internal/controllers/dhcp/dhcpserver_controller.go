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
	"fmt"
	"strings"

	"github.com/go-logr/logr"
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
	ipv4Pool, ipv6Pool, res, stop, err := r.lookupIPPools(ctx, dhcpServer)
	if err != nil {
		return res, fmt.Errorf("looking up IPPools: %w", err)
	}
	if stop {
		return res, nil
	}

	// Phase 2.
	// Ensure the Gateway IP Address has a Static Lease,
	// so it's not allocated by the DHCP Server.
	if stop, err := r.ensureGatewayIPLeases(ctx, dhcpServer, ipv4Pool, ipv6Pool); err != nil {
		return res, fmt.Errorf("ensuring gateway IPLeases: %w", err)
	} else if stop {
		// we are retriggered by watching IPLeases
		return res, nil
	}

	// Phase 3.
	// Lease IP addresses for the DHCP Server
	ipv4DHCPLease, ipv6DHCPLease, stop, err := r.ensureIPLeases(ctx, dhcpServer, ipv4Pool, ipv6Pool)
	if err != nil {
		return res, fmt.Errorf("ensuring dhcp server IPLeases: %w", err)
	} else if stop {
		// we are retriggered by watching IPLeases
		return res, nil
	}

	// Phase 4.
	// Reconcile DHCP Server deployment
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dhcpServer.Name + "-dhcp-server",
			Namespace: dhcpServer.Namespace,
			Labels:    map[string]string{},
		},
	}
	addCommonLabels(sa.Labels, dhcpServer)
	if err := controllerutil.SetControllerReference(dhcpServer, sa, r.Scheme); err != nil {
		return res, err
	}
	_, err = reconcile.ServiceAccount(ctx, r.Client, sa)
	if err != nil {
		return res, fmt.Errorf("reconcile ServiceAccount: %w", err)
	}

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sa.Name,
			Namespace: sa.Namespace,
			Labels:    map[string]string{},
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
	if err := controllerutil.SetControllerReference(dhcpServer, roleBinding, r.Scheme); err != nil {
		return res, err
	}
	addCommonLabels(roleBinding.Labels, dhcpServer)
	_, err = reconcile.RoleBinding(ctx, r.Client, roleBinding)
	if err != nil {
		return res, fmt.Errorf("reconcile RoleBinding: %w", err)
	}

	deploy, stop, err := r.ensureDeployment(
		ctx, dhcpServer, ipv4DHCPLease, ipv6DHCPLease, ipv6Pool, sa)
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
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

func (r *DHCPServerReconciler) lookupIPPools(
	ctx context.Context, dhcpServer *dhcpv1alpha1.DHCPServer,
) (
	ipv4Pool *ipamv1alpha1.IPv4Pool,
	ipv6Pool *ipamv1alpha1.IPv6Pool,
	res ctrl.Result,
	stop bool,
	err error,
) {
	var poolsNotFound []string

	if dhcpServer.Spec.IPv4 != nil {
		// IPv4 enabled
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
				poolsNotFound, fmt.Sprintf("IPv4:%q", dhcpServer.Spec.IPv4.Pool.Name))

		default:
			return nil, nil, res, false, err
		}
	}

	if dhcpServer.Spec.IPv6 != nil {
		// IPv6 enabled
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
				poolsNotFound, fmt.Sprintf("IPv6:%q", dhcpServer.Spec.IPv6.Pool.Name))

		default:
			return nil, nil, res, false, err
		}
	}

	if len(poolsNotFound) > 0 {
		dhcpServer.Status.Phase = "IPPoolNotFound"
		dhcpServer.Status.ObservedGeneration = dhcpServer.Generation
		meta.SetStatusCondition(&dhcpServer.Status.Conditions, metav1.Condition{
			Type:   dhcpv1alpha1.Available,
			Status: metav1.ConditionFalse,
			Reason: "IPPoolNotFound",
			Message: "IPPools " +
				strings.Join(poolsNotFound, ", ") + " not found",
			ObservedGeneration: dhcpServer.Generation,
		})
		res.Requeue = true // check later
		return nil, nil, res, true, r.Status().Update(ctx, dhcpServer)
	}
	return
}

func (r *DHCPServerReconciler) ensureGatewayIPLeases(
	ctx context.Context, dhcpServer *dhcpv1alpha1.DHCPServer,
	ipv4Pool *ipamv1alpha1.IPv4Pool, ipv6Pool *ipamv1alpha1.IPv6Pool,
) (stop bool, err error) {
	// Ensure IPv*Lease objects exist
	ipv4GatewayLease, err := r.ensureIPv4GatewayLease(ctx, dhcpServer, ipv4Pool)
	if err != nil {
		return false, fmt.Errorf("ensuring IPv4Lease for Gateway IP: %w", err)
	}
	ipv6GatewayLease, err := r.ensureIPv6GatewayLease(ctx, dhcpServer, ipv6Pool)
	if err != nil {
		return false, fmt.Errorf("ensuring IPv6Lease for Gateway IP: %w", err)
	}

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
		return true, r.Status().Update(ctx, dhcpServer)
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
		return true, r.Status().Update(ctx, dhcpServer)
	}
	return false, nil
}

func (r *DHCPServerReconciler) ensureIPLeases(
	ctx context.Context, dhcpServer *dhcpv1alpha1.DHCPServer,
	ipv4Pool *ipamv1alpha1.IPv4Pool, ipv6Pool *ipamv1alpha1.IPv6Pool,
) (
	ipv4Lease *ipamv1alpha1.IPv4Lease,
	ipv6Lease *ipamv1alpha1.IPv6Lease,
	stop bool,
	err error,
) {
	ipv4Lease, err = r.ensureIPv4Lease(ctx, dhcpServer, ipv4Pool)
	if err != nil {
		return nil, nil, false, fmt.Errorf("ensuring IPv4Lease for DHCP server: %w", err)
	}
	ipv6Lease, err = r.ensureIPv6Lease(ctx, dhcpServer, ipv6Pool)
	if err != nil {
		return nil, nil, false, fmt.Errorf("ensuring IPv6Lease for DHCP server: %w", err)
	}

	var (
		unreadyIPLease []string
		failedIPLease  []string
	)
	if ipv4Lease != nil {
		switch {
		case meta.IsStatusConditionTrue(
			ipv4Lease.Status.Conditions, ipamv1alpha1.IPLeaseBound):
			dhcpServer.Status.IPv4Address = ipv4Lease.Status.Address

		case meta.IsStatusConditionFalse(
			ipv4Lease.Status.Conditions, ipamv1alpha1.IPLeaseBound):
			failedIPLease = append(failedIPLease, ipv4Lease.Name)

		default:
			unreadyIPLease = append(unreadyIPLease, ipv4Lease.Name)
		}
	}

	if ipv6Lease != nil {
		switch {
		case meta.IsStatusConditionTrue(
			ipv6Lease.Status.Conditions, ipamv1alpha1.IPLeaseBound):
			dhcpServer.Status.IPv6Address = ipv6Lease.Status.Address

		case meta.IsStatusConditionFalse(
			ipv6Lease.Status.Conditions, ipamv1alpha1.IPLeaseBound):
			failedIPLease = append(failedIPLease, ipv6Lease.Name)

		default:
			unreadyIPLease = append(unreadyIPLease, ipv6Lease.Name)
		}
	}

	if len(failedIPLease) > 0 {
		// Failed Leasing
		meta.SetStatusCondition(&dhcpServer.Status.Conditions, metav1.Condition{
			Type:   dhcpv1alpha1.Available,
			Status: metav1.ConditionFalse,
			Reason: "UnboundIP",
			Message: fmt.Sprintf(
				"Could not lease IPs for DHCP server: %s",
				strings.Join(failedIPLease, ", ")),
			ObservedGeneration: dhcpServer.Generation,
		})
		dhcpServer.Status.ObservedGeneration = dhcpServer.Generation
		dhcpServer.Status.Phase = "Failed"
		return nil, nil, true, r.Status().Update(ctx, dhcpServer)
	}

	if len(unreadyIPLease) > 0 {
		// Unready Lease
		meta.SetStatusCondition(&dhcpServer.Status.Conditions, metav1.Condition{
			Type:   dhcpv1alpha1.Available,
			Status: metav1.ConditionFalse,
			Reason: "UnboundIP",
			Message: fmt.Sprintf(
				"Pending IPLease on DHCP server: %s",
				strings.Join(unreadyIPLease, ", ")),
			ObservedGeneration: dhcpServer.Generation,
		})
		dhcpServer.Status.ObservedGeneration = dhcpServer.Generation
		dhcpServer.Status.Phase = "Pending"
		return nil, nil, true, r.Status().Update(ctx, dhcpServer)
	}
	return
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

func (r *DHCPServerReconciler) ensureDeployment(
	ctx context.Context,
	dhcpServer *dhcpv1alpha1.DHCPServer,
	ipv4Lease *ipamv1alpha1.IPv4Lease,
	ipv6Lease *ipamv1alpha1.IPv6Lease,
	ipv6Pool *ipamv1alpha1.IPv6Pool,
	sa *corev1.ServiceAccount,
) (_ *appsv1.Deployment, stop bool, err error) {
	deploy, err := deployment(r.Scheme, dhcpServer, ipv4Lease, ipv6Lease, ipv6Pool, sa)
	if err != nil {
		return nil, false, fmt.Errorf("preparing Deployment: %w", err)
	}

	currentDeploy, err := reconcile.Deployment(ctx, r.Client, deploy)
	if err != nil {
		return nil, false, fmt.Errorf("reconciling Deployment: %w", err)
	}
	return currentDeploy, false, nil
}
