//go:build integration

/*
Copyright 2024.

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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	valkeyv1 "github.com/turboslop/valkey-operator/api/v1"
	globalcfg "github.com/turboslop/valkey-operator/cfg"
)

var _ = Describe("Valkey Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		valkey := &valkeyv1.Valkey{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Valkey")
			err := k8sClient.Get(ctx, typeNamespacedName, valkey)
			if err != nil && errors.IsNotFound(err) {
				resource := &valkeyv1.Valkey{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: valkeyv1.ValkeySpec{
						AnonymousAuth: true,
						Shards:        2,
						Replicas:      1,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &valkeyv1.Valkey{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Valkey")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ValkeyReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(32),
				GlobalConfig: &globalcfg.Config{
					ValkeyImage:  "valkey:test",
					SidecarImage: "valkey-sidecar:test",
					Nodes:        1,
				},
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())
			Expect(result.RequeueAfter).NotTo(BeZero())

			configMap := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, configMap)).To(Succeed())
			Expect(configMap.Data).To(HaveKey("valkey.conf"))
			Expect(configMap.Data).To(HaveKey("ping_readiness_local.sh"))
			Expect(configMap.Data).To(HaveKey("ping_liveness_local.sh"))

			service := &corev1.Service{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, service)).To(Succeed())
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(service.Spec.Ports).To(ContainElement(HaveField("Port", int32(ValkeyPort))))
			Expect(service.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/instance", resourceName))

			headlessService := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-headless",
				Namespace: "default",
			}, headlessService)).To(Succeed())
			Expect(headlessService.Spec.ClusterIP).To(Equal("None"))
			Expect(headlessService.Spec.PublishNotReadyAddresses).To(BeTrue())

			serviceAccount := &corev1.ServiceAccount{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, serviceAccount)).To(Succeed())
			Expect(serviceAccount.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", Valkey))

			pdb := &policyv1.PodDisruptionBudget{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, pdb)).To(Succeed())
			Expect(pdb.Spec.MaxUnavailable).NotTo(BeNil())
			Expect(pdb.Spec.MaxUnavailable.IntVal).To(Equal(int32(1)))

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, sts)).To(Succeed())
			Expect(sts.Spec.Replicas).NotTo(BeNil())
			Expect(*sts.Spec.Replicas).To(Equal(int32(4)))
			Expect(sts.Spec.ServiceName).To(Equal(resourceName + "-headless"))
			Expect(sts.Spec.Template.Spec.ServiceAccountName).To(Equal(resourceName))
			Expect(sts.Spec.Template.Spec.Containers).NotTo(BeEmpty())
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("valkey:test"))
		})
	})
})
