// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

// Code generated by client-gen. DO NOT EDIT.

package v2

import (
	context "context"

	ciliumiov2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	scheme "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// CiliumNetworkPoliciesGetter has a method to return a CiliumNetworkPolicyInterface.
// A group's client should implement this interface.
type CiliumNetworkPoliciesGetter interface {
	CiliumNetworkPolicies(namespace string) CiliumNetworkPolicyInterface
}

// CiliumNetworkPolicyInterface has methods to work with CiliumNetworkPolicy resources.
type CiliumNetworkPolicyInterface interface {
	Create(ctx context.Context, ciliumNetworkPolicy *ciliumiov2.CiliumNetworkPolicy, opts v1.CreateOptions) (*ciliumiov2.CiliumNetworkPolicy, error)
	Update(ctx context.Context, ciliumNetworkPolicy *ciliumiov2.CiliumNetworkPolicy, opts v1.UpdateOptions) (*ciliumiov2.CiliumNetworkPolicy, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, ciliumNetworkPolicy *ciliumiov2.CiliumNetworkPolicy, opts v1.UpdateOptions) (*ciliumiov2.CiliumNetworkPolicy, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*ciliumiov2.CiliumNetworkPolicy, error)
	List(ctx context.Context, opts v1.ListOptions) (*ciliumiov2.CiliumNetworkPolicyList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *ciliumiov2.CiliumNetworkPolicy, err error)
	CiliumNetworkPolicyExpansion
}

// ciliumNetworkPolicies implements CiliumNetworkPolicyInterface
type ciliumNetworkPolicies struct {
	*gentype.ClientWithList[*ciliumiov2.CiliumNetworkPolicy, *ciliumiov2.CiliumNetworkPolicyList]
}

// newCiliumNetworkPolicies returns a CiliumNetworkPolicies
func newCiliumNetworkPolicies(c *CiliumV2Client, namespace string) *ciliumNetworkPolicies {
	return &ciliumNetworkPolicies{
		gentype.NewClientWithList[*ciliumiov2.CiliumNetworkPolicy, *ciliumiov2.CiliumNetworkPolicyList](
			"ciliumnetworkpolicies",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *ciliumiov2.CiliumNetworkPolicy { return &ciliumiov2.CiliumNetworkPolicy{} },
			func() *ciliumiov2.CiliumNetworkPolicyList { return &ciliumiov2.CiliumNetworkPolicyList{} },
		),
	}
}
