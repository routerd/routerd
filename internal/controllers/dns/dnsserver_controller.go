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

package dns

import (
	"context"
	"encoding/json"
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
	utilpointer "k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dnsv1alpha1 "routerd.net/routerd/apis/dns/v1alpha1"
	ipamv1alpha1 "routerd.net/routerd/apis/ipam/v1alpha1"
	"routerd.net/routerd/internal/reconcile"
	"routerd.net/routerd/internal/version"
)

// DNSServerReconciler reconciles a DNSServer object
type DNSServerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

const networksAnnotations = "k8s.v1.cni.cncf.io/networks"

func (r *DNSServerReconciler) Reconcile(
	ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	dnsServer := &dnsv1alpha1.DNSServer{}
	if err = r.Get(ctx, req.NamespacedName, dnsServer); err != nil {
		return res, client.IgnoreNotFound(err)
	}

	// Phase 1.
	// Lookup IPPools
	ipv4Pool, ipv6Pool, res, stop, err := r.lookupIPPools(ctx, dnsServer)
	if err != nil {
		return res, fmt.Errorf("looking up IPPools: %w", err)
	}
	if stop {
		return res, nil
	}

	// Phase 2.
	// Lease IP addresses for the DNS Server
	ipv4DNSLease, ipv6DNSLease, stop, err := r.ensureIPLeases(ctx, dnsServer, ipv4Pool, ipv6Pool)
	if err != nil {
		return res, fmt.Errorf("ensuring dns server IPLeases: %w", err)
	} else if stop {
		// we are retriggered by watching IPLeases
		return res, nil
	}

	// Phase 3.
	// Reconcile DNS Server deployment
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dnsServer.Name + "-dns-server",
			Namespace: dnsServer.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(dnsServer, sa, r.Scheme); err != nil {
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
			Name:     "routerd-dns-role",
		},
	}
	if err := controllerutil.SetControllerReference(dnsServer, roleBinding, r.Scheme); err != nil {
		return res, err
	}
	_, err = reconcile.RoleBinding(ctx, r.Client, roleBinding)
	if err != nil {
		return res, fmt.Errorf("reconcile RoleBinding: %w", err)
	}

	// ConfigMap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dnsServer.Name + "-dns-server",
			Namespace: dnsServer.Namespace,
			Labels:    map[string]string{},
		},
		Data: map[string]string{
			"Corefile": corefile(dnsServer),
		},
	}
	addCommonLabels(configMap.Labels, dnsServer)
	if err := controllerutil.SetControllerReference(dnsServer, configMap, r.Scheme); err != nil {
		return res, err
	}
	currentConfigMap, err := reconcile.ConfigMap(ctx, r.Client, configMap)
	if err != nil {
		return res, fmt.Errorf("reconcile ConfigMap: %w", err)
	}

	// Deployment
	var netconfigIPs []string
	if ipv4DNSLease != nil {
		netconfigIPs = append(netconfigIPs, ipv4DNSLease.Status.Address)
	}
	if ipv6DNSLease != nil {
		netconfigIPs = append(netconfigIPs, ipv6DNSLease.Status.Address)
	}
	netconfig := []map[string]interface{}{
		{
			"name": dnsServer.Spec.NetworkAttachment.NetworkAttachmentDefinition.Name,
			"ips":  netconfigIPs,
		},
	}
	netconfigJson, _ := json.MarshalIndent(netconfig, "", "  ")

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dnsServer.Name + "-dns-server",
			Namespace: dnsServer.Namespace,
			Labels:    map[string]string{},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilpointer.Int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
			Strategy: appsv1.DeploymentStrategy{
				// We only want a single instance running at any given time.
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
					Annotations: map[string]string{
						networksAnnotations:           string(netconfigJson),
						"routerd.net/config-revision": currentConfigMap.ResourceVersion,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: sa.Name,
					Containers: []corev1.Container{
						{
							Name:            "dns-server",
							Image:           "quay.io/routerd/routerd-dns:" + version.Version,
							ImagePullPolicy: corev1.PullAlways,
							Args:            []string{"-conf", "/config/Corefile"},
							Env: []corev1.EnvVar{
								{
									Name: "KUBERNETES_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name:  "ROUTERD_DNSSERVER_NAME",
									Value: dnsServer.Name,
								},
								{
									Name:  "ROUTERD_DNSSERVER_SELECTOR",
									Value: dnsServer.Spec.RecordSelector.String(),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-corefile",
									ReadOnly:  true,
									MountPath: "/config/",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config-corefile",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: configMap.Name,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	addCommonLabels(deploy.Labels, dnsServer)
	addCommonLabels(deploy.Spec.Selector.MatchLabels, dnsServer)
	addCommonLabels(deploy.Spec.Template.ObjectMeta.Labels, dnsServer)
	if err := controllerutil.SetControllerReference(dnsServer, deploy, r.Scheme); err != nil {
		return res, err
	}

	currentDeploy, err := reconcile.Deployment(ctx, r.Client, deploy)
	if err != nil {
		return res, fmt.Errorf("reconciling Deployment: %w", err)
	}

	dnsServer.Status.ObservedGeneration = dnsServer.Generation
	if currentDeploy.Status.AvailableReplicas == currentDeploy.Status.Replicas {
		dnsServer.Status.Phase = "Ready"
		meta.SetStatusCondition(&dnsServer.Status.Conditions, metav1.Condition{
			Type:               dnsv1alpha1.DNSServerAvailable,
			Status:             metav1.ConditionTrue,
			Reason:             "DeploymentReady",
			Message:            "DNS Server Deployment ready",
			ObservedGeneration: deploy.Generation,
		})
	} else {
		dnsServer.Status.Phase = "Unready"
		meta.SetStatusCondition(&dnsServer.Status.Conditions, metav1.Condition{
			Type:               dnsv1alpha1.DNSServerAvailable,
			Status:             metav1.ConditionFalse,
			Reason:             "DeploymentUnready",
			Message:            "DNS Server Deployment is not ready",
			ObservedGeneration: deploy.Generation,
		})
	}
	if err = r.Status().Update(ctx, dnsServer); err != nil {
		return
	}
	return
}

func (r *DNSServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1alpha1.DNSServer{}).
		Owns(&ipamv1alpha1.IPv4Lease{}).
		Owns(&ipamv1alpha1.IPv6Lease{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}

func (r *DNSServerReconciler) lookupIPPools(
	ctx context.Context, dnsServer *dnsv1alpha1.DNSServer,
) (
	ipv4Pool *ipamv1alpha1.IPv4Pool,
	ipv6Pool *ipamv1alpha1.IPv6Pool,
	res ctrl.Result,
	stop bool,
	err error,
) {
	var poolsNotFound []string

	if len(dnsServer.Spec.NetworkAttachment.IPv4Pool.Name) > 0 {
		// IPv4 enabled
		ippool := &ipamv1alpha1.IPv4Pool{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      dnsServer.Spec.NetworkAttachment.IPv4Pool.Name,
			Namespace: dnsServer.Namespace,
		}, ippool)
		switch {
		case err == nil:
			ipv4Pool = ippool

		case errors.IsNotFound(err):
			poolsNotFound = append(
				poolsNotFound, fmt.Sprintf("IPv4:%q", dnsServer.Spec.NetworkAttachment.IPv4Pool.Name))

		default:
			return nil, nil, res, false, err
		}
	}

	if len(dnsServer.Spec.NetworkAttachment.IPv6Pool.Name) > 0 {
		// IPv6 enabled
		ippool := &ipamv1alpha1.IPv6Pool{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      dnsServer.Spec.NetworkAttachment.IPv4Pool.Name,
			Namespace: dnsServer.Namespace,
		}, ippool)
		switch {
		case err == nil:
			ipv6Pool = ippool

		case errors.IsNotFound(err):
			poolsNotFound = append(
				poolsNotFound, fmt.Sprintf("IPv6:%q", dnsServer.Spec.NetworkAttachment.IPv4Pool.Name))

		default:
			return nil, nil, res, false, err
		}
	}

	if len(poolsNotFound) > 0 {
		dnsServer.Status.Phase = "IPPoolNotFound"
		dnsServer.Status.ObservedGeneration = dnsServer.Generation
		meta.SetStatusCondition(&dnsServer.Status.Conditions, metav1.Condition{
			Type:   dnsv1alpha1.DNSServerAvailable,
			Status: metav1.ConditionFalse,
			Reason: "IPPoolNotFound",
			Message: "IPPools " +
				strings.Join(poolsNotFound, ", ") + " not found",
			ObservedGeneration: dnsServer.Generation,
		})
		res.Requeue = true // check later
		return nil, nil, res, true, r.Status().Update(ctx, dnsServer)
	}
	return
}

func (r *DNSServerReconciler) ensureIPLeases(
	ctx context.Context, dnsServer *dnsv1alpha1.DNSServer,
	ipv4Pool *ipamv1alpha1.IPv4Pool, ipv6Pool *ipamv1alpha1.IPv6Pool,
) (
	ipv4Lease *ipamv1alpha1.IPv4Lease,
	ipv6Lease *ipamv1alpha1.IPv6Lease,
	stop bool,
	err error,
) {
	ipv4Lease, err = r.ensureIPv4Lease(ctx, dnsServer, ipv4Pool)
	if err != nil {
		return nil, nil, false, fmt.Errorf("ensuring IPv4Lease for DHCP server: %w", err)
	}
	ipv6Lease, err = r.ensureIPv6Lease(ctx, dnsServer, ipv6Pool)
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
			dnsServer.Status.IPv4Address = ipv4Lease.Status.Address

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
			dnsServer.Status.IPv6Address = ipv6Lease.Status.Address

		case meta.IsStatusConditionFalse(
			ipv6Lease.Status.Conditions, ipamv1alpha1.IPLeaseBound):
			failedIPLease = append(failedIPLease, ipv6Lease.Name)

		default:
			unreadyIPLease = append(unreadyIPLease, ipv6Lease.Name)
		}
	}

	if len(failedIPLease) > 0 {
		// Failed Leasing
		meta.SetStatusCondition(&dnsServer.Status.Conditions, metav1.Condition{
			Type:   dnsv1alpha1.DNSServerAvailable,
			Status: metav1.ConditionFalse,
			Reason: "UnboundIP",
			Message: fmt.Sprintf(
				"Could not lease IPs for DHCP server: %s",
				strings.Join(failedIPLease, ", ")),
			ObservedGeneration: dnsServer.Generation,
		})
		dnsServer.Status.ObservedGeneration = dnsServer.Generation
		dnsServer.Status.Phase = "Failed"
		return nil, nil, true, r.Status().Update(ctx, dnsServer)
	}

	if len(unreadyIPLease) > 0 {
		// Unready Lease
		meta.SetStatusCondition(&dnsServer.Status.Conditions, metav1.Condition{
			Type:   dnsv1alpha1.DNSServerAvailable,
			Status: metav1.ConditionFalse,
			Reason: "UnboundIP",
			Message: fmt.Sprintf(
				"Pending IPLease on DHCP server: %s",
				strings.Join(unreadyIPLease, ", ")),
			ObservedGeneration: dnsServer.Generation,
		})
		dnsServer.Status.ObservedGeneration = dnsServer.Generation
		dnsServer.Status.Phase = "Pending"
		return nil, nil, true, r.Status().Update(ctx, dnsServer)
	}
	return
}

// ensures the DHCP Server Pod has a Lease for it's own IP address.
func (r *DNSServerReconciler) ensureIPv4Lease(
	ctx context.Context,
	dnsServer *dnsv1alpha1.DNSServer,
	ippool *ipamv1alpha1.IPv4Pool,
) (_ *ipamv1alpha1.IPv4Lease, err error) {
	if len(dnsServer.Spec.NetworkAttachment.IPv4Pool.Name) == 0 {
		return nil, nil
	}

	iplease := &ipamv1alpha1.IPv4Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ippool.Name + "-dns",
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
	addCommonLabels(iplease.Labels, dnsServer)
	if err := controllerutil.SetControllerReference(dnsServer, iplease, r.Scheme); err != nil {
		return nil, err
	}

	currentIPLease, err := reconcile.IPv4Lease(ctx, r.Client, iplease)
	if err != nil {
		return nil, fmt.Errorf("reconciling gateway IPLease: %w", err)
	}
	return currentIPLease, nil
}

// ensures the DHCP Server Pod has a Lease for it's own IP address.
func (r *DNSServerReconciler) ensureIPv6Lease(
	ctx context.Context,
	dnsServer *dnsv1alpha1.DNSServer,
	ippool *ipamv1alpha1.IPv6Pool,
) (_ *ipamv1alpha1.IPv6Lease, err error) {
	if len(dnsServer.Spec.NetworkAttachment.IPv6Pool.Name) == 0 {
		return nil, nil
	}

	iplease := &ipamv1alpha1.IPv6Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ippool.Name + "-dns",
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
	addCommonLabels(iplease.Labels, dnsServer)
	if err := controllerutil.SetControllerReference(dnsServer, iplease, r.Scheme); err != nil {
		return nil, err
	}

	currentIPLease, err := reconcile.IPv6Lease(ctx, r.Client, iplease)
	if err != nil {
		return nil, fmt.Errorf("reconciling gateway IPLease: %w", err)
	}
	return currentIPLease, nil
}

const (
	commonNameLabel      = "app.kubernetes.io/name"
	commonComponentLabel = "app.kubernetes.io/component"
	commonInstanceLabel  = "app.kubernetes.io/instance"
	commonManagedByLabel = "app.kubernetes.io/managed-by"
)

func addCommonLabels(labels map[string]string, dnsServer *dnsv1alpha1.DNSServer) {
	if labels == nil {
		return
	}

	labels[commonNameLabel] = "routerd-dns"
	labels[commonComponentLabel] = "dns-server"
	labels[commonManagedByLabel] = "routerd"
	labels[commonInstanceLabel] = dnsServer.Name
}

func corefile(dnsServer *dnsv1alpha1.DNSServer) string {
	config := `. {
	cancel
	ready
	health
	errors
	log
	any
	loadbalance
	routerd
	loop
`

	if f := dnsServer.Spec.Forward; f != nil {
		config += "\tforward " + f.From + " " + strings.Join(f.To, " ") + " {\n"
		config += "\t\tpolicy " + strings.ToLower(string(f.Policy)) + "\n"
		if len(f.Except) > 0 {
			config += "\t\texcept " + strings.Join(f.Except, " ") + "\n"
		}
		config += "\t}\n"
	}

	if c := dnsServer.Spec.Cache; c != nil && len(c.Zones) > 0 {
		config += "\tcache " + c.TTL.String() + " " + strings.Join(c.Zones, " ") + "\n"
	}
	config += "}\n"

	return strings.Replace(config, "\t", "  ", -1)
}
