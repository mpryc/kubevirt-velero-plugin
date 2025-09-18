/*
 * This file is part of the Kubevirt Velero Plugin project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2025 Red Hat, Inc.
 *
 */

package plugin

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kvcore "kubevirt.io/api/core/v1"
	"kubevirt.io/kubevirt-velero-plugin/pkg/util"
)

func TestPVCBackupItemAction(t *testing.T) {
	testCases := []struct {
		name               string
		pvc                *corev1api.PersistentVolumeClaim
		backup             *velerov1.Backup
		expectedVMNames    []string
		expectedLabels     map[string]string
		expectedAnns       map[string]string
		setupCache         bool
		cacheData          map[string][]string // pvcName -> vmNames
	}{
		{
			name: "PVC used by single VM should be labeled",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
				},
			},
			backup: &velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"persistentvolumeclaims", "virtualmachines"},
				},
			},
			setupCache: true,
			cacheData: map[string][]string{
				"test-pvc": {"vm1"},
			},
			expectedLabels: map[string]string{
				util.VMNameLabel: "vm1",
			},
			expectedAnns: map[string]string{
				util.VMNameLabelAddedAnnotation: "true",
			},
		},
		{
			name: "PVC used by multiple VMs should skip labeling (not supported)",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-pvc",
					Namespace: "test-ns",
				},
			},
			backup: &velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"persistentvolumeclaims", "virtualmachines"},
				},
			},
			setupCache: true,
			cacheData: map[string][]string{
				"shared-pvc": {"vm1", "vm2", "vm3"},
			},
			expectedLabels: nil, // No labels added when multiple VMs
			expectedAnns:   nil, // No annotations added when multiple VMs
		},
		{
			name: "PVC not used by any VM should not be labeled",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unused-pvc",
					Namespace: "test-ns",
				},
			},
			backup: &velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"persistentvolumeclaims", "virtualmachines"},
				},
			},
			setupCache: true,
			cacheData:     map[string][]string{}, // No PVC mappings
			expectedLabels: nil,
			expectedAnns:   nil,
		},
		{
			name: "PVC should not be labeled when VMs not included in backup",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
				},
			},
			backup: &velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"persistentvolumeclaims"}, // No VMs
				},
			},
			setupCache:    false, // Cache won't be populated
			expectedLabels: nil,
			expectedAnns:   nil,
		},
		{
			name: "PVC should preserve existing labels and annotations",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app": "myapp",
					},
					Annotations: map[string]string{
						"description": "test pvc",
					},
				},
			},
			backup: &velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"persistentvolumeclaims", "virtualmachines"},
				},
			},
			setupCache: true,
			cacheData: map[string][]string{
				"test-pvc": {"vm1"},
			},
			expectedLabels: map[string]string{
				"app":            "myapp",
				util.VMNameLabel: "vm1",
			},
			expectedAnns: map[string]string{
				"description":                   "test pvc",
				util.VMNameLabelAddedAnnotation: "true",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			action := NewPVCBackupItemAction(logrus.New())

			// Setup cache if needed
			if tc.setupCache {
				action.pvcToVMsCache[tc.pvc.Namespace] = tc.cacheData
			}

			// Convert PVC to unstructured
			pvcUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(tc.pvc)
			if !assert.NoError(t, err) {
			return
		}
			item := &unstructured.Unstructured{Object: pvcUnstructured}

			// Execute the action
			result, additionalItems, err := action.Execute(item, tc.backup)
			if !assert.NoError(t, err) {
			return
		}
			assert.Empty(t, additionalItems)

			// Convert result back to PVC
			var resultPVC corev1api.PersistentVolumeClaim
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(result.UnstructuredContent(), &resultPVC)
			if !assert.NoError(t, err) {
			return
		}

			// Verify labels
			if tc.expectedLabels == nil {
				assert.Nil(t, resultPVC.Labels)
			} else {
				assert.Equal(t, tc.expectedLabels, resultPVC.Labels)
			}

			// Verify annotations
			if tc.expectedAnns == nil {
				// Should preserve original annotations
				assert.Equal(t, tc.pvc.Annotations, resultPVC.Annotations)
			} else {
				assert.Equal(t, tc.expectedAnns, resultPVC.Annotations)
			}
		})
	}
}

func TestGetPVCsUsedByVM(t *testing.T) {
	testCases := []struct {
		name        string
		vm          *kvcore.VirtualMachine
		expectedPVCs []string
	}{
		{
			name: "VM with PVC volumes",
			vm: &kvcore.VirtualMachine{
				Spec: kvcore.VirtualMachineSpec{
					Template: &kvcore.VirtualMachineInstanceTemplateSpec{
						Spec: kvcore.VirtualMachineInstanceSpec{
							Volumes: []kvcore.Volume{
								{
									VolumeSource: kvcore.VolumeSource{
										PersistentVolumeClaim: &kvcore.PersistentVolumeClaimVolumeSource{
											PersistentVolumeClaimVolumeSource: corev1api.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc1",
											},
										},
									},
								},
								{
									VolumeSource: kvcore.VolumeSource{
										PersistentVolumeClaim: &kvcore.PersistentVolumeClaimVolumeSource{
											PersistentVolumeClaimVolumeSource: corev1api.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc2",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedPVCs: []string{"pvc1", "pvc2"},
		},
		{
			name: "VM with DataVolume references",
			vm: &kvcore.VirtualMachine{
				Spec: kvcore.VirtualMachineSpec{
					Template: &kvcore.VirtualMachineInstanceTemplateSpec{
						Spec: kvcore.VirtualMachineInstanceSpec{
							Volumes: []kvcore.Volume{
								{
									VolumeSource: kvcore.VolumeSource{
										DataVolume: &kvcore.DataVolumeSource{
											Name: "dv1",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedPVCs: []string{"dv1"},
		},
		{
			name: "VM with DataVolumeTemplates",
			vm: &kvcore.VirtualMachine{
				Spec: kvcore.VirtualMachineSpec{
					DataVolumeTemplates: []kvcore.DataVolumeTemplateSpec{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "dv-template1",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "dv-template2",
							},
						},
					},
				},
			},
			expectedPVCs: []string{"dv-template1", "dv-template2"},
		},
		{
			name: "VM with mixed volume types",
			vm: &kvcore.VirtualMachine{
				Spec: kvcore.VirtualMachineSpec{
					Template: &kvcore.VirtualMachineInstanceTemplateSpec{
						Spec: kvcore.VirtualMachineInstanceSpec{
							Volumes: []kvcore.Volume{
								{
									VolumeSource: kvcore.VolumeSource{
										PersistentVolumeClaim: &kvcore.PersistentVolumeClaimVolumeSource{
											PersistentVolumeClaimVolumeSource: corev1api.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc1",
											},
										},
									},
								},
								{
									VolumeSource: kvcore.VolumeSource{
										DataVolume: &kvcore.DataVolumeSource{
											Name: "dv1",
										},
									},
								},
								{
									VolumeSource: kvcore.VolumeSource{
										ConfigMap: &kvcore.ConfigMapVolumeSource{},
									},
								},
							},
						},
					},
					DataVolumeTemplates: []kvcore.DataVolumeTemplateSpec{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "dv-template1",
							},
						},
					},
				},
			},
			expectedPVCs: []string{"pvc1", "dv1", "dv-template1"},
		},
		{
			name: "VM with no PVC volumes",
			vm: &kvcore.VirtualMachine{
				Spec: kvcore.VirtualMachineSpec{
					Template: &kvcore.VirtualMachineInstanceTemplateSpec{
						Spec: kvcore.VirtualMachineInstanceSpec{
							Volumes: []kvcore.Volume{
								{
									VolumeSource: kvcore.VolumeSource{
										ConfigMap: &kvcore.ConfigMapVolumeSource{},
									},
								},
								{
									VolumeSource: kvcore.VolumeSource{
										Secret: &kvcore.SecretVolumeSource{},
									},
								},
							},
						},
					},
				},
			},
			expectedPVCs: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			action := NewPVCBackupItemAction(logrus.New())
			result := action.getPVCsUsedByVM(tc.vm)
			assert.Equal(t, tc.expectedPVCs, result)
		})
	}
}

func TestAddVMLabelsToBackupContent(t *testing.T) {
	testCases := []struct {
		name            string
		pvc             *corev1api.PersistentVolumeClaim
		vmNames         []string
		expectedLabels  map[string]string
		expectedAnns    map[string]string
	}{
		{
			name: "Single VM name",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
				},
			},
			vmNames: []string{"vm1"},
			expectedLabels: map[string]string{
				util.VMNameLabel: "vm1",
			},
			expectedAnns: map[string]string{
				util.VMNameLabelAddedAnnotation: "true",
			},
		},
		{
			name: "Multiple VM names (not supported - no labels added)",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
				},
			},
			vmNames:        []string{"vm1", "vm2", "vm3"},
			expectedLabels: nil, // No labels when multiple VMs
			expectedAnns:   nil, // No annotations when multiple VMs
		},
		{
			name: "Preserve existing labels and annotations",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app": "myapp",
					},
					Annotations: map[string]string{
						"description": "test pvc",
					},
				},
			},
			vmNames: []string{"vm1"},
			expectedLabels: map[string]string{
				"app":            "myapp",
				util.VMNameLabel: "vm1",
			},
			expectedAnns: map[string]string{
				"description":                   "test pvc",
				util.VMNameLabelAddedAnnotation: "true",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			action := NewPVCBackupItemAction(logrus.New())
			action.addVMLabelsToBackupContent(tc.pvc, tc.vmNames)

			assert.Equal(t, tc.expectedLabels, tc.pvc.Labels)
			assert.Equal(t, tc.expectedAnns, tc.pvc.Annotations)
		})
	}
}

func TestAddVMLabelsUserManagedScenarios(t *testing.T) {
	testCases := []struct {
		name            string
		pvc             *corev1api.PersistentVolumeClaim
		vmNames         []string
		expectedLabels  map[string]string
		expectedAnns    map[string]string
		expectWarning   bool
	}{
		{
			name: "Preserve user-managed VM label when it differs from discovered VM",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "user-vm", // User-managed (no tracking annotation)
						"app":            "myapp",
					},
					Annotations: map[string]string{
						"description": "test pvc",
					},
				},
			},
			vmNames: []string{"discovered-vm"}, // Different from user label
			expectedLabels: map[string]string{
				"app":            "myapp",
				util.VMNameLabel: "discovered-vm", // Plugin overwrites but preserves original
			},
			expectedAnns: map[string]string{
				"description":                     "test pvc",
				util.VMNameLabelAddedAnnotation:   "true",
				util.VMNameOriginalAnnotation:     "user-vm", // Original preserved
			},
			expectWarning: false, // No warning - preservation approach
		},
		{
			name: "Track user-managed VM label when it matches discovered VM",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "vm1", // User-managed but matches discovery
						"app":            "myapp",
					},
					Annotations: map[string]string{
						"description": "test pvc",
					},
				},
			},
			vmNames: []string{"vm1"}, // Same as user label
			expectedLabels: map[string]string{
				"app":            "myapp",
				util.VMNameLabel: "vm1", // Label unchanged
			},
			expectedAnns: map[string]string{
				"description":                   "test pvc",
				util.VMNameLabelAddedAnnotation: "true", // Now tracking this label
			},
			expectWarning: false, // No warning since labels match
		},
		{
			name: "Update plugin-managed VM label",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "old-vm", // Plugin-managed
						"app":            "myapp",
					},
					Annotations: map[string]string{
						util.VMNameLabelAddedAnnotation: "true", // Plugin tracking
						"description":                   "test pvc",
					},
				},
			},
			vmNames: []string{"new-vm"}, // Updated VM
			expectedLabels: map[string]string{
				"app":            "myapp",
				util.VMNameLabel: "new-vm", // Should update to new VM
			},
			expectedAnns: map[string]string{
				"description":                   "test pvc",
				util.VMNameLabelAddedAnnotation: "true",
			},
			expectWarning: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			action := NewPVCBackupItemAction(logrus.New())
			action.addVMLabelsToBackupContent(tc.pvc, tc.vmNames)

			assert.Equal(t, tc.expectedLabels, tc.pvc.Labels)
			assert.Equal(t, tc.expectedAnns, tc.pvc.Annotations)
		})
	}
}

func TestPVCBackupItemActionAppliesTo(t *testing.T) {
	action := NewPVCBackupItemAction(logrus.New())

	selector, err := action.AppliesTo()

	assert.NoError(t, err)
	assert.Equal(t, []string{"PersistentVolumeClaim"}, selector.IncludedResources)
	assert.Empty(t, selector.ExcludedResources)
}
