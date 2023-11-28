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

package functional_test

import (
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("OVNDBCluster controller", func() {

	When("A OVNDBCluster instance is created", func() {
		var OVNDBClusterName types.NamespacedName

		BeforeEach(func() {
			name := fmt.Sprintf("ovndbcluster-%s", uuid.New().String())
			instance := CreateOVNDBCluster(namespace, name, GetDefaultOVNDBClusterSpec())
			OVNDBClusterName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should have the Spec fields initialized", func() {
			OVNDBCluster := GetOVNDBCluster(OVNDBClusterName)
			Expect(*(OVNDBCluster.Spec.Replicas)).Should(Equal(int32(1)))
			Expect(OVNDBCluster.Spec.LogLevel).Should(Equal("info"))
			Expect(OVNDBCluster.Spec.DBType).Should(Equal(v1beta1.NBDBType))
		})

		It("should have the Status fields initialized", func() {
			OVNDBCluster := GetOVNDBCluster(OVNDBClusterName)
			Expect(OVNDBCluster.Status.Hash).To(BeEmpty())
			Expect(OVNDBCluster.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetOVNDBCluster(OVNDBClusterName).Finalizers
			}, timeout, interval).Should(ContainElement("OVNDBCluster"))
		})

		DescribeTable("should not create the config map",
			func(cmName string) {
				cm := types.NamespacedName{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-%s", OVNDBClusterName.Name, cmName),
				}
				th.AssertConfigMapDoesNotExist(cm)
			},
			Entry("config-data CM", "config-data"),
			Entry("scripts CM", "scripts"),
		)
		DescribeTable("should eventually create the config maps with OwnerReferences set",
			func(cmName string) {
				cm := types.NamespacedName{
					Namespace: OVNDBClusterName.Namespace,
					Name:      fmt.Sprintf("%s-%s", OVNDBClusterName.Name, cmName),
				}
				Eventually(func() corev1.ConfigMap {

					return *th.GetConfigMap(cm)
				}, timeout, interval).ShouldNot(BeNil())
				// Check OwnerReferences set correctly for the Config Map
				Expect(th.GetConfigMap(cm).ObjectMeta.OwnerReferences[0].Name).To(Equal(OVNDBClusterName.Name))
				Expect(th.GetConfigMap(cm).ObjectMeta.OwnerReferences[0].Kind).To(Equal("OVNDBCluster"))
			},
			Entry("config-data CM", "config-data"),
			Entry("scripts CM", "scripts"),
		)

		It("should create a scripts ConfigMap with namespace from CR", func() {
			cm := types.NamespacedName{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-%s", OVNDBClusterName.Name, "scripts"),
			}
			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(cm)
			}, timeout, interval).ShouldNot(BeNil())

			Expect(th.GetConfigMap(cm).Data["setup.sh"]).Should(
				ContainSubstring(fmt.Sprintf("NAMESPACE=\"%s\"", namespace)))
		})
	})

	When("A OVNDBCluster instance is created with debug on", func() {
		BeforeEach(func() {
			name := fmt.Sprintf("ovndbcluster-%s", uuid.New().String())
			spec := GetDefaultOVNDBClusterSpec()
			spec["debug"] = map[string]interface{}{
				"service": true,
			}
			instance := CreateOVNDBCluster(namespace, name, spec)
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("Container commands to include debug commands", func() {
			ssName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-nb",
			}
			ss := th.GetStatefulSet(ssName)
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(ss.Spec.Template.Spec.Containers[0].LivenessProbe.Exec.Command).To(
				Equal([]string{"/bin/true"}))
			Expect(ss.Spec.Template.Spec.Containers[0].Args[1]).Should(ContainSubstring("sleep infinity"))
			Expect(ss.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(
				Equal([]string{"/bin/true"}))
			Expect(ss.Spec.Template.Spec.Containers[0].Lifecycle.PostStart.Exec.Command).To(
				Equal([]string{"/bin/true"}))
		})
	})
})
