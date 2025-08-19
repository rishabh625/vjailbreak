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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
)

var _ = Describe("RDMDisk Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		rdmdisk := &vjailbreakv1alpha1.RDMDisk{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind RDMDisk")
			err := k8sClient.Get(ctx, typeNamespacedName, rdmdisk)
			if err != nil && errors.IsNotFound(err) {
				resource := &vjailbreakv1alpha1.RDMDisk{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &vjailbreakv1alpha1.RDMDisk{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance RDMDisk")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &RDMDiskReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When validating RDMDisk fields", func() {
		It("should return an error if required fields are missing", func() {
			rdmDisk := &vjailbreakv1alpha1.RDMDisk{
				Spec: vjailbreakv1alpha1.RDMDiskSpec{
					OpenstackVolumeRef: vjailbreakv1alpha1.OpenstackVolumeRef{},
				},
			}

			err := ValidateRDMDiskFields(rdmDisk)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("OpenstackVolumeRef.source is required"))
		})

		It("should pass validation if all required fields are present", func() {
			rdmDisk := &vjailbreakv1alpha1.RDMDisk{
				Spec: vjailbreakv1alpha1.RDMDiskSpec{
					OpenstackVolumeRef: vjailbreakv1alpha1.OpenstackVolumeRef{
						VolumeRef: map[string]string{
							"sourceKey": "sourceValue",
						},
						CinderBackendPool: "valid-pool",
						VolumeType:        "valid-type",
					},
					DiskName: "valid-disk",
				},
			}

			err := ValidateRDMDiskFields(rdmDisk)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
