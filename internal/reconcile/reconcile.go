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
