/*
Copyright 2022.

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

package v1beta1

import (
	"context"
	"fmt"

	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

//
// GetDBEndpoints - get DB Endpoints
//
func GetDBEndpoints(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
	labelSelector map[string]string,
) (map[string]string, error) {
	ovnDBList := &OVNDBClusterList{}

	listOpts := []client.ListOption{
		client.InNamespace(namespace),
	}

	if len(labelSelector) > 0 {
		labels := client.MatchingLabels(labelSelector)
		listOpts = append(listOpts, labels)
	}

	err := h.GetClient().List(ctx, ovnDBList, listOpts...)
	if err != nil {
		return nil, err
	}

	if len(ovnDBList.Items) > 2 {
		return nil, fmt.Errorf("more then two OVNDBCluster object found in namespace %s", namespace)
	}

	if len(ovnDBList.Items) == 0 {
		return nil, k8s_errors.NewNotFound(
			appsv1.Resource("OVNDBCluster"),
			fmt.Sprintf("No OVNDBCluster object found in namespace %s", namespace),
		)
	}
	DBEndpointsMap := make(map[string]string)
	for _, ovndb := range ovnDBList.Items {
		if  ovndb.Status.DBAddress != "" {
			DBEndpointsMap[ovndb.Spec.DBType] = ovndb.Status.DBAddress
		} else {
			return DBEndpointsMap, fmt.Errorf("DBEndpoint not ready yet for %s", ovndb.Spec.DBType)
		}
	}
	return DBEndpointsMap, nil
}
