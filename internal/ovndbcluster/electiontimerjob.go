/*
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

// Package ovndbcluster provides functionality for managing OVN database cluster components
package ovndbcluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// UpdateRuntimeConfigSignal - create/update a ConfigMap to signal runtime configuration changes
func UpdateRuntimeConfigSignal(
	ctx context.Context,
	k8sClient client.Client,
	instance *ovnv1.OVNDBCluster,
	serviceName string,
) error {

	// Create a ConfigMap that signals the database pods to reconfigure
	configMapName := fmt.Sprintf("%s-runtime-config-signal", serviceName)

	// Determine parameters based on DB type
	dbType := strings.ToLower(instance.Spec.DBType)
	dbScheme := "ptcp"
	dbPort := 6641
	if instance.Spec.TLS.Enabled() {
		dbScheme = "pssl"
	}
	if instance.Spec.DBType == ovnv1.SBDBType {
		dbPort = 6642
	}

	configData := map[string]string{
		"db-type":         dbType,
		"election-timer":  fmt.Sprintf("%d", instance.Spec.ElectionTimer),
		"inactivity-probe": fmt.Sprintf("%d", instance.Spec.InactivityProbe),
		"log-level":       instance.Spec.LogLevel,
		"db-scheme":       dbScheme,
		"db-port":         fmt.Sprintf("%d", dbPort),
		"timestamp":       fmt.Sprintf("%d", time.Now().Unix()),
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: instance.Namespace,
			Labels: map[string]string{
				"app":     serviceName,
				"service": serviceName,
				"type":    "runtime-config-signal",
			},
		},
		Data: configData,
	}

	// Create or update the ConfigMap
	existing := &corev1.ConfigMap{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      configMapName,
		Namespace: instance.Namespace,
	}, existing)

	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Create new ConfigMap
			return k8sClient.Create(ctx, configMap)
		}
		return err
	}

	// Update existing ConfigMap
	existing.Data = configData
	return k8sClient.Update(ctx, existing)
}

// getOVNDBClusterPods returns the pods belonging to the OVNDBCluster StatefulSet
func getOVNDBClusterPods(
	ctx context.Context,
	k8sClient client.Client,
	instance *ovnv1.OVNDBCluster,
	serviceName string,
) (*corev1.PodList, error) {
	ovnPods := &corev1.PodList{}

	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels{
			"service": serviceName,
		},
	}

	err := k8sClient.List(ctx, ovnPods, listOpts...)
	if err != nil {
		return nil, err
	}

	return ovnPods, nil
}