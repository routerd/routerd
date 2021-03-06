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

package ipam

import (
	"context"
	"testing"
	"time"

	"github.com/metal-stack/go-ipam"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"inet.af/netaddr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	ipamv1alpha1 "routerd.net/routerd/apis/ipam/v1alpha1"
	"routerd.net/routerd/internal/controllers/ipam/adapter"
	"routerd.net/routerd/internal/testutil"
)

func TestIPLeaseReconciler(t *testing.T) {
	ippool := adapter.AdaptIPPool(&ipamv1alpha1.IPv4Pool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pool",
			Namespace: "default",
			UID:       types.UID("1234"),
		},
		Spec: ipamv1alpha1.IPv4PoolSpec{
			CIDR: "192.0.2.0/24",
			LeaseDuration: &metav1.Duration{
				Duration: time.Hour,
			},
		},
	})
	ippoolNN := types.NamespacedName{
		Name:      ippool.GetName(),
		Namespace: ippool.GetNamespace(),
	}

	ipv4 := &ipam.IP{
		IP: netaddr.MustParseIP("192.0.2.23"),
	}

	t.Run("static fixed ip", func(t *testing.T) {
		iplease := adapter.AdaptIPLease(&ipamv1alpha1.IPv4Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lease",
				Namespace: "default",
			},
			Spec: ipamv1alpha1.IPv4LeaseSpec{
				Pool: ipamv1alpha1.LocalObjectReference{
					Name: ippool.GetName(),
				},
				Type: ipamv1alpha1.IPLeaseTypeStatic,
				Static: &ipamv1alpha1.IPLeaseStatic{
					Address: "192.0.2.23",
				},
			},
		})
		ipleaseNN := types.NamespacedName{
			Name:      iplease.GetName(),
			Namespace: iplease.GetNamespace(),
		}

		ipam := &ipamMock{}
		ipamCache := &ipamCacheMock{}
		ipam.
			On("AcquireSpecificIP", ippool.GetCIDR(), iplease.GetSpecStaticAddress()).
			Return(ipv4, nil)

		ipamCache.
			On("Get", ippool).
			Return(ipam, true)

		c := testutil.NewClient()
		c.On("Get", mock.Anything, ipleaseNN, mock.AnythingOfType("*v1alpha1.IPv4Lease")).
			Run(func(args mock.Arguments) {
				ipv4lease := args.Get(2).(*ipamv1alpha1.IPv4Lease)
				*ipv4lease = *(iplease.ClientObject().(*ipamv1alpha1.IPv4Lease))
			}).
			Return(nil)
		c.On("Get", mock.Anything, ippoolNN, mock.AnythingOfType("*v1alpha1.IPv4Pool")).
			Run(func(args mock.Arguments) {
				ipv4pool := args.Get(2).(*ipamv1alpha1.IPv4Pool)
				*ipv4pool = *(ippool.ClientObject().(*ipamv1alpha1.IPv4Pool))
			}).
			Return(nil)
		c.On("Update",
			mock.Anything, mock.AnythingOfType("*v1alpha1.IPv4Lease"), mock.Anything).
			Return(nil)
		var leaseStatusUpdate *ipamv1alpha1.IPv4Lease
		c.StatusMock.
			On("Update",
				mock.Anything, mock.AnythingOfType("*v1alpha1.IPv4Lease"), mock.Anything).
			Run(func(args mock.Arguments) {
				leaseStatusUpdate = args.Get(1).(*ipamv1alpha1.IPv4Lease)
			}).
			Return(nil)

		r := &IPLeaseReconciler{
			Client:      c,
			Log:         testutil.NewLogger(t),
			IPAMCache:   ipamCache,
			IPPoolType:  adapter.AdaptIPPool(&ipamv1alpha1.IPv4Pool{}),
			IPLeaseType: adapter.AdaptIPLease(&ipamv1alpha1.IPv4Lease{}),
		}

		ctx := context.Background()
		res, err := r.Reconcile(ctx, ctrl.Request{
			NamespacedName: ipleaseNN,
		})
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.Empty(t, res.RequeueAfter)

		assert.Equal(t,
			ipv4.IP.String()+"/24", leaseStatusUpdate.Status.Address)
		assert.Nil(t, leaseStatusUpdate.Status.LeaseDuration)
	})

	t.Run("static dynamic ip", func(t *testing.T) {
		iplease := adapter.AdaptIPLease(&ipamv1alpha1.IPv4Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lease",
				Namespace: "default",
			},
			Spec: ipamv1alpha1.IPv4LeaseSpec{
				Pool: ipamv1alpha1.LocalObjectReference{
					Name: ippool.GetName(),
				},
				Type: ipamv1alpha1.IPLeaseTypeStatic,
			},
		})
		ipleaseNN := types.NamespacedName{
			Name:      iplease.GetName(),
			Namespace: iplease.GetNamespace(),
		}

		ipam := &ipamMock{}
		ipamCache := &ipamCacheMock{}
		ipam.
			On("AcquireIP", ippool.GetCIDR()).
			Return(ipv4, nil)

		ipamCache.
			On("Get", ippool).
			Return(ipam, true)

		ipamCache.
			On("Get", ippool).
			Return(ipam, true)

		c := testutil.NewClient()
		c.On("Get", mock.Anything, ipleaseNN, mock.AnythingOfType("*v1alpha1.IPv4Lease")).
			Run(func(args mock.Arguments) {
				ipv4lease := args.Get(2).(*ipamv1alpha1.IPv4Lease)
				*ipv4lease = *(iplease.ClientObject().(*ipamv1alpha1.IPv4Lease))
			}).
			Return(nil)

		c.On("Get", mock.Anything, ippoolNN, mock.AnythingOfType("*v1alpha1.IPv4Pool")).
			Run(func(args mock.Arguments) {
				ipv4pool := args.Get(2).(*ipamv1alpha1.IPv4Pool)
				*ipv4pool = *(ippool.ClientObject().(*ipamv1alpha1.IPv4Pool))
			}).
			Return(nil)
		c.On("Update",
			mock.Anything, mock.AnythingOfType("*v1alpha1.IPv4Lease"), mock.Anything).
			Return(nil)
		var leaseStatusUpdate *ipamv1alpha1.IPv4Lease
		c.StatusMock.
			On("Update",
				mock.Anything, mock.AnythingOfType("*v1alpha1.IPv4Lease"), mock.Anything).
			Run(func(args mock.Arguments) {
				leaseStatusUpdate = args.Get(1).(*ipamv1alpha1.IPv4Lease)
			}).
			Return(nil)

		r := &IPLeaseReconciler{
			Client:      c,
			Log:         testutil.NewLogger(t),
			IPAMCache:   ipamCache,
			IPPoolType:  adapter.AdaptIPPool(&ipamv1alpha1.IPv4Pool{}),
			IPLeaseType: adapter.AdaptIPLease(&ipamv1alpha1.IPv4Lease{}),
		}

		ctx := context.Background()
		res, err := r.Reconcile(ctx, ctrl.Request{
			NamespacedName: ipleaseNN,
		})
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.Empty(t, res.RequeueAfter)

		assert.Equal(t,
			ipv4.IP.String()+"/24", leaseStatusUpdate.Status.Address)
		assert.Nil(t, leaseStatusUpdate.Status.LeaseDuration)
	})

	t.Run("dynamic", func(t *testing.T) {
		iplease := adapter.AdaptIPLease(&ipamv1alpha1.IPv4Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lease",
				Namespace: "default",
			},
			Spec: ipamv1alpha1.IPv4LeaseSpec{
				Pool: ipamv1alpha1.LocalObjectReference{
					Name: ippool.GetName(),
				},
				Type: ipamv1alpha1.IPLeaseTypeDynamic,
			},
		})
		ipleaseNN := types.NamespacedName{
			Name:      iplease.GetName(),
			Namespace: iplease.GetNamespace(),
		}

		ipam := &ipamMock{}
		ipamCache := &ipamCacheMock{}
		ipam.
			On("AcquireIP", ippool.GetCIDR()).
			Return(ipv4, nil)

		ipamCache.
			On("Get", ippool).
			Return(ipam, true)

		ipamCache.
			On("Get", ippool).
			Return(ipam, true)

		c := testutil.NewClient()
		c.On("Get", mock.Anything, ipleaseNN, mock.AnythingOfType("*v1alpha1.IPv4Lease")).
			Run(func(args mock.Arguments) {
				ipv4lease := args.Get(2).(*ipamv1alpha1.IPv4Lease)
				*ipv4lease = *(iplease.ClientObject().(*ipamv1alpha1.IPv4Lease))
			}).
			Return(nil)

		c.On("Get", mock.Anything, ippoolNN, mock.AnythingOfType("*v1alpha1.IPv4Pool")).
			Run(func(args mock.Arguments) {
				ipv4pool := args.Get(2).(*ipamv1alpha1.IPv4Pool)
				*ipv4pool = *(ippool.ClientObject().(*ipamv1alpha1.IPv4Pool))
			}).
			Return(nil)
		c.On("Update",
			mock.Anything, mock.AnythingOfType("*v1alpha1.IPv4Lease"), mock.Anything).
			Return(nil)
		var leaseStatusUpdate *ipamv1alpha1.IPv4Lease
		c.StatusMock.
			On("Update",
				mock.Anything, mock.AnythingOfType("*v1alpha1.IPv4Lease"), mock.Anything).
			Run(func(args mock.Arguments) {
				leaseStatusUpdate = args.Get(1).(*ipamv1alpha1.IPv4Lease)
			}).
			Return(nil)

		r := &IPLeaseReconciler{
			Client:      c,
			Log:         testutil.NewLogger(t),
			IPAMCache:   ipamCache,
			IPPoolType:  adapter.AdaptIPPool(&ipamv1alpha1.IPv4Pool{}),
			IPLeaseType: adapter.AdaptIPLease(&ipamv1alpha1.IPv4Lease{}),
		}

		ctx := context.Background()
		res, err := r.Reconcile(ctx, ctrl.Request{
			NamespacedName: ipleaseNN,
		})
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		leaseDuration, _ := ippool.GetSpecLeaseDuration()
		assert.Equal(t, leaseDuration, res.RequeueAfter)

		assert.Equal(t,
			ipv4.IP.String()+"/24", leaseStatusUpdate.Status.Address)
		if assert.NotNil(t, leaseStatusUpdate.Status.LeaseDuration) {
			assert.Equal(
				t, leaseDuration,
				leaseStatusUpdate.Status.LeaseDuration.Duration)
		}
	})
}
