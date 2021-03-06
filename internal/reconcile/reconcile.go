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

package reconcile

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ipamv1alpha1 "routerd.net/routerd/apis/ipam/v1alpha1"
)

// reconciles an IPv4Lease and returns the current object as observed.
func IPv4Lease(
	ctx context.Context,
	c client.Client, iplease *ipamv1alpha1.IPv4Lease,
) (*ipamv1alpha1.IPv4Lease, error) {
	currentIPLease := &ipamv1alpha1.IPv4Lease{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      iplease.Name,
		Namespace: iplease.Namespace,
	}, currentIPLease)
	if errors.IsNotFound(err) {
		return iplease, c.Create(ctx, iplease)
	}
	if err != nil {
		return nil, err
	}

	return currentIPLease, nil
}

// reconciles an IPv6Lease and returns the current object as observed.
func IPv6Lease(
	ctx context.Context,
	c client.Client, iplease *ipamv1alpha1.IPv6Lease,
) (*ipamv1alpha1.IPv6Lease, error) {
	currentIPLease := &ipamv1alpha1.IPv6Lease{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      iplease.Name,
		Namespace: iplease.Namespace,
	}, currentIPLease)
	if errors.IsNotFound(err) {
		return iplease, c.Create(ctx, iplease)
	}
	if err != nil {
		return nil, err
	}

	return currentIPLease, nil
}

func ServiceAccount(
	ctx context.Context,
	c client.Client, sa *corev1.ServiceAccount,
) (*corev1.ServiceAccount, error) {
	currentSA := &corev1.ServiceAccount{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      sa.Name,
		Namespace: sa.Namespace,
	}, currentSA)
	if errors.IsNotFound(err) {
		return sa, c.Create(ctx, sa)
	}
	if err != nil {
		return nil, err
	}

	return currentSA, nil
}

func RoleBinding(
	ctx context.Context,
	c client.Client, sa *rbacv1.RoleBinding,
) (*rbacv1.RoleBinding, error) {
	currentSA := &rbacv1.RoleBinding{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      sa.Name,
		Namespace: sa.Namespace,
	}, currentSA)
	if errors.IsNotFound(err) {
		return sa, c.Create(ctx, sa)
	}
	if err != nil {
		return nil, err
	}

	return currentSA, nil
}

func Deployment(
	ctx context.Context,
	c client.Client, deploy *appsv1.Deployment,
) (currentDeploy *appsv1.Deployment, err error) {
	currentDeploy = &appsv1.Deployment{}
	err = c.Get(ctx, types.NamespacedName{
		Name:      deploy.Name,
		Namespace: deploy.Namespace,
	}, currentDeploy)
	if errors.IsNotFound(err) {
		return deploy, c.Create(ctx, deploy)
	}
	if err != nil {
		return nil, err
	}

	if equality.Semantic.DeepDerivative(deploy.Spec, currentDeploy.Spec) {
		// objects are equal
		return currentDeploy, nil
	}
	// update
	currentDeploy.Spec = deploy.Spec
	return currentDeploy, c.Update(ctx, currentDeploy)
}
