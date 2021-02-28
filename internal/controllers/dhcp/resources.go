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
	netv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dhcpv1alpha1 "routerd.net/routerd/apis/dhcp/v1alpha1"
)

const networksAnnotations = "k8s.v1.cni.cncf.io/networks"

func deployment(
	scheme *runtime.Scheme,
	dhcpServer *dhcpv1alpha1.DHCPServer,
	nad *netv1.NetworkAttachmentDefinition,
) (*appsv1.Deployment, error) {
	env := []corev1.EnvVar{
		{Name: "DHCP_BIND_INTERFACE", Value: "net1"},
		{Name: "DHCP_SERVER_NAME", Value: dhcpServer.Name},
		{
			Name: "KUBERNETES_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
	}

	if dhcpServer.Spec.IPv4 != nil {
		env = append(env, corev1.EnvVar{
			Name:  "DHCP_ENABLE_IPv4",
			Value: "True",
		})
	}
	if dhcpServer.Spec.IPv6 != nil {
		env = append(env, corev1.EnvVar{
			Name:  "DHCP_ENABLE_IPv6",
			Value: "True",
		})
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dhcpServer.Name,
			Namespace: dhcpServer.Namespace,
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
						networksAnnotations: nad.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							ImagePullPolicy: corev1.PullAlways,
							Name:            "dhcp-server",
							Image:           "quay.io/routerd/kube-dhcp:implementation-10cfe61",
							Env:             env,
						},
					},
				},
			},
		},
	}
	addCommonLabels(deploy.Labels, dhcpServer)
	addCommonLabels(deploy.Spec.Selector.MatchLabels, dhcpServer)
	addCommonLabels(deploy.Spec.Template.ObjectMeta.Labels, dhcpServer)

	if err := controllerutil.SetControllerReference(dhcpServer, deploy, scheme); err != nil {
		return nil, err
	}

	return deploy, nil
}

const (
	commonNameLabel      = "app.kubernetes.io/name"
	commonComponentLabel = "app.kubernetes.io/component"
	commonInstanceLabel  = "app.kubernetes.io/instance"
	commonManagedByLabel = "app.kubernetes.io/managed-by"
)

func addCommonLabels(labels map[string]string, dhcpServer *dhcpv1alpha1.DHCPServer) {
	if labels == nil {
		return
	}

	labels[commonNameLabel] = "kube-dhcp"
	labels[commonComponentLabel] = "dhcp-server"
	labels[commonManagedByLabel] = "kube-dhcp-operator"
	labels[commonInstanceLabel] = dhcpServer.Name
}
