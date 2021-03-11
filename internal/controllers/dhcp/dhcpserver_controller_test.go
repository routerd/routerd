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
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	dhcpv1alpha1 "routerd.net/routerd/apis/dhcp/v1alpha1"
	ipamv1alpha1 "routerd.net/routerd/apis/ipam/v1alpha1"
	"routerd.net/routerd/internal/testutil"
)

var (
	dhcpServer = &dhcpv1alpha1.DHCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dhcp-server1",
			Namespace: "dhcp-ns",
		},
		Spec: dhcpv1alpha1.DHCPServerSpec{
			IPv4: &dhcpv1alpha1.DHCPServerIPv4{
				Pool: dhcpv1alpha1.LocalObjectReference{
					Name: "ipv4-pool",
				},
			},
			IPv6: &dhcpv1alpha1.DHCPServerIPv6{
				Pool: dhcpv1alpha1.LocalObjectReference{
					Name: "ipv6-pool",
				},
			},
		},
	}
	notFoundErr = &errors.StatusError{
		ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Code:   http.StatusNotFound,
			Reason: metav1.StatusReasonNotFound,
		},
	}
	testScheme = runtime.NewScheme()
)

func TestDHCPServerController_lookupIPPools(t *testing.T) {

	t.Run("should get both pools", func(t *testing.T) {
		c := testutil.NewClient()
		r := &DHCPServerReconciler{
			Client: c,
		}

		c.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		ctx := context.Background()

		ipv4Pool, ipv6Pool, res, stop, err := r.lookupIPPools(ctx, dhcpServer)
		require.NoError(t, err)
		assert.NotNil(t, ipv4Pool)
		assert.NotNil(t, ipv6Pool)
		assert.False(t, res.Requeue)
		assert.Empty(t, res.RequeueAfter)
		assert.False(t, stop)

		c.AssertCalled(t, "Get", mock.Anything, types.NamespacedName{
			Name:      "ipv4-pool",
			Namespace: dhcpServer.Namespace,
		}, mock.AnythingOfType("*v1alpha1.IPv4Pool"))
		c.AssertCalled(t, "Get", mock.Anything, types.NamespacedName{
			Name:      "ipv6-pool",
			Namespace: dhcpServer.Namespace,
		}, mock.AnythingOfType("*v1alpha1.IPv6Pool"))
	})

	t.Run("should report not found pools", func(t *testing.T) {
		tests := []struct {
			name            string
			setup           func(c *testutil.Client)
			expectedMessage string
		}{
			{
				name: "IPv6Pool not found",
				setup: func(c *testutil.Client) {
					c.
						On("Get",
							mock.Anything,
							mock.Anything,
							mock.AnythingOfType("*v1alpha1.IPv4Pool"),
						).
						Return(nil)
					c.
						On("Get",
							mock.Anything,
							mock.Anything,
							mock.AnythingOfType("*v1alpha1.IPv6Pool"),
						).
						Return(notFoundErr)
				},
				expectedMessage: `IPPools IPv6:"ipv6-pool" not found`,
			},
			{
				name: "IPv4Pool not found",
				setup: func(c *testutil.Client) {
					c.
						On("Get",
							mock.Anything,
							mock.Anything,
							mock.AnythingOfType("*v1alpha1.IPv6Pool"),
						).
						Return(nil)
					c.
						On("Get",
							mock.Anything,
							mock.Anything,
							mock.AnythingOfType("*v1alpha1.IPv4Pool"),
						).
						Return(notFoundErr)
				},
				expectedMessage: `IPPools IPv4:"ipv4-pool" not found`,
			},
			{
				name: "Both not found",
				setup: func(c *testutil.Client) {
					c.
						On("Get",
							mock.Anything,
							mock.Anything,
							mock.AnythingOfType("*v1alpha1.IPv6Pool"),
						).
						Return(notFoundErr)
					c.
						On("Get",
							mock.Anything,
							mock.Anything,
							mock.AnythingOfType("*v1alpha1.IPv4Pool"),
						).
						Return(notFoundErr)
				},
				expectedMessage: `IPPools IPv4:"ipv4-pool", IPv6:"ipv6-pool" not found`,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				c := testutil.NewClient()
				r := &DHCPServerReconciler{
					Client: c,
				}

				test.setup(c)

				var updatedDHCPServer *dhcpv1alpha1.DHCPServer
				c.StatusMock.
					On("Update",
						mock.Anything,
						mock.AnythingOfType("*v1alpha1.DHCPServer"),
						mock.Anything,
					).
					Run(func(args mock.Arguments) {
						updatedDHCPServer = args.Get(1).(*dhcpv1alpha1.DHCPServer)
					}).
					Return(nil)

				ctx := context.Background()
				_, _, _, _, err := r.lookupIPPools(ctx, dhcpServer)
				require.NoError(t, err)

				if assert.NotNil(t, updatedDHCPServer) {
					available := meta.FindStatusCondition(
						updatedDHCPServer.Status.Conditions, dhcpv1alpha1.Available)
					require.NotNil(t, available)

					assert.Equal(t, metav1.ConditionFalse, available.Status)
					assert.Equal(t, "IPPoolNotFound", available.Reason)
					assert.Equal(t, test.expectedMessage, available.Message)
				}
			})
		}
	})
}

func TestDHCPServerController_ensureIPLeases(t *testing.T) {
	ipv4Pool := &ipamv1alpha1.IPv4Pool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ipv4-pool",
			Namespace: dhcpServer.Namespace,
		},
	}
	ipv6Pool := &ipamv1alpha1.IPv6Pool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ipv6-pool",
			Namespace: dhcpServer.Namespace,
		},
	}

	ipv4Lease := &ipamv1alpha1.IPv4Lease{
		Status: ipamv1alpha1.IPv4LeaseStatus{
			Conditions: []metav1.Condition{
				{
					Type:   ipamv1alpha1.IPLeaseBound,
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	ipv6Lease := &ipamv1alpha1.IPv6Lease{
		Status: ipamv1alpha1.IPv6LeaseStatus{
			Conditions: []metav1.Condition{
				{
					Type:   ipamv1alpha1.IPLeaseBound,
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	c := testutil.NewClient()
	c.
		On("Get", mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.IPv4Lease")).
		Return(notFoundErr)
	c.
		On("Get", mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.IPv6Lease")).
		Return(notFoundErr)
	c.
		On("Create", mock.Anything, mock.AnythingOfType("*v1alpha1.IPv4Lease"), mock.Anything).
		Run(func(args mock.Arguments) {
			l := args.Get(1).(*ipamv1alpha1.IPv4Lease)
			*l = *ipv4Lease
		}).
		Return(nil)
	c.
		On("Create", mock.Anything, mock.AnythingOfType("*v1alpha1.IPv6Lease"), mock.Anything).
		Run(func(args mock.Arguments) {
			l := args.Get(1).(*ipamv1alpha1.IPv6Lease)
			*l = *ipv6Lease
		}).
		Return(nil)

	r := &DHCPServerReconciler{
		Client: c,
		Scheme: testScheme,
	}
	ipv4Lease, ipv6Lease, stop, err := r.ensureIPLeases(
		context.Background(), dhcpServer, ipv4Pool, ipv6Pool)
	require.NoError(t, err)
	assert.NotNil(t, ipv4Lease)
	assert.NotNil(t, ipv6Lease)
	assert.False(t, stop)
}

func init() {
	_ = dhcpv1alpha1.AddToScheme(testScheme)
	_ = ipamv1alpha1.AddToScheme(testScheme)
}
