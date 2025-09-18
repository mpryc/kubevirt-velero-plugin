package plugin

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"kubevirt.io/kubevirt-velero-plugin/pkg/util"
)

func TestPvcRestoreExecute(t *testing.T) {
	testCases := []struct {
		name  string
		input velero.RestoreItemActionExecuteInput
	}{
		{"Skip the unfinished PVC ",
			velero.RestoreItemActionExecuteInput{
				Item: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "PersistentVolumeClaim",
						"metadata": map[string]interface{}{
							"name": "test-pvc",
							"annotations": map[string]string{
								AnnInProgress: "test-pvc",
							},
							"ownerReferences": []interface{}{
								map[string]interface{}{
									"apiVersion": "cdi.kubevirt.io/v1beta1",
									"kind":       "DataVolume",
									"name":       "test-datavolume",
								},
							},
						},
						"spec": map[string]interface{}{},
					},
				},
			},
		},
	}

	logrus.SetLevel(logrus.ErrorLevel)
	action := NewPVCRestoreItemAction(logrus.StandardLogger())
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			item, _ := action.Execute(&tc.input)
			assert.True(t, item.SkipRestore)
		})
	}
}

func TestPVCRestoreItemActionVMLabeling(t *testing.T) {
	testCases := []struct {
		name            string
		pvc             *corev1api.PersistentVolumeClaim
		expectedLabels  map[string]string
		expectedAnns    map[string]string
		shouldSkip      bool
	}{
		{
			name: "Remove VM labels added by plugin",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "vm1",
						"app":            "myapp", // Should be preserved
					},
					Annotations: map[string]string{
						util.VMNameLabelAddedAnnotation: "true",
						"description":                   "test pvc", // Should be preserved
					},
				},
			},
			expectedLabels: map[string]string{
				"app": "myapp", // Preserved
			},
			expectedAnns: map[string]string{
				"description": "test pvc", // Preserved
			},
		},
		{
			name: "Don't remove labels not added by plugin (no annotation)",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "vm1", // No tracking annotation
						"app":            "myapp",
					},
					Annotations: map[string]string{
						"description": "test pvc",
					},
				},
			},
			expectedLabels: map[string]string{
				util.VMNameLabel: "vm1", // Should be preserved
				"app":            "myapp",
			},
			expectedAnns: map[string]string{
				"description": "test pvc",
			},
		},
		{
			name: "Don't remove labels when annotation is false",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "vm1",
						"app":            "myapp",
					},
					Annotations: map[string]string{
						util.VMNameLabelAddedAnnotation: "false", // Not added by plugin
						"description":                   "test pvc",
					},
				},
			},
			expectedLabels: map[string]string{
				util.VMNameLabel: "vm1", // Should be preserved
				"app":            "myapp",
			},
			expectedAnns: map[string]string{
				util.VMNameLabelAddedAnnotation: "false",
				"description":                   "test pvc",
			},
		},
		{
			name: "Handle PVC with in-progress annotation (should skip)",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "vm1",
					},
					Annotations: map[string]string{
						AnnInProgress:                   "test-dv",
						util.VMNameLabelAddedAnnotation: "true",
					},
				},
			},
			shouldSkip: true,
		},
		{
			name: "Handle PVC with no labels or annotations",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
				},
			},
			expectedLabels: nil,
			expectedAnns:   nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			action := NewPVCRestoreItemAction(logrus.New())

			// Convert PVC to unstructured
			pvcUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(tc.pvc)
			if !assert.NoError(t, err) {
			return
		}
			item := &unstructured.Unstructured{Object: pvcUnstructured}

			input := &velero.RestoreItemActionExecuteInput{
				Item: item,
			}

			// Execute the action
			result, err := action.Execute(input)
			if !assert.NoError(t, err) {
			return
		}

			if tc.shouldSkip {
				assert.True(t, result.SkipRestore)
				return
			}

			assert.False(t, result.SkipRestore)

			// Convert result back to PVC
			var resultPVC corev1api.PersistentVolumeClaim
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(result.UpdatedItem.UnstructuredContent(), &resultPVC)
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
				assert.Nil(t, resultPVC.Annotations)
			} else {
				assert.Equal(t, tc.expectedAnns, resultPVC.Annotations)
			}
		})
	}
}

func TestBackupRestoreCycle(t *testing.T) {
	testCases := []struct {
		name                    string
		originalPVC            *corev1api.PersistentVolumeClaim
		discoveredVMs          []string
		expectedAfterBackup    map[string]string
		expectedAfterRestore   map[string]string
		expectedAnnsAfterBackup map[string]string
		expectedAnnsAfterRestore map[string]string
	}{
		{
			name: "User-managed label preserved through backup/restore cycle",
			originalPVC: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "user-vm", // User-managed original label
						"app":            "myapp",
					},
					Annotations: map[string]string{
						"description": "test pvc",
					},
				},
			},
			discoveredVMs: []string{"discovered-vm"}, // Plugin discovers different VM
			expectedAfterBackup: map[string]string{
				"app":            "myapp",
				util.VMNameLabel: "discovered-vm", // Plugin overwrites but preserves original
			},
			expectedAfterRestore: map[string]string{
				"app":            "myapp",
				util.VMNameLabel: "user-vm", // User label restored to original
			},
			expectedAnnsAfterBackup: map[string]string{
				"description":                     "test pvc",
				util.VMNameLabelAddedAnnotation:   "true",
				util.VMNameOriginalAnnotation:     "user-vm", // Original preserved
			},
			expectedAnnsAfterRestore: map[string]string{
				"description": "test pvc",
				// Plugin annotations removed
			},
		},
		{
			name: "Plugin-managed labels properly removed in restore",
			originalPVC: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app": "myapp", // Only user labels
					},
					Annotations: map[string]string{
						"description": "test pvc",
					},
				},
			},
			discoveredVMs: []string{"vm1"}, // Plugin discovers VM
			expectedAfterBackup: map[string]string{
				"app":            "myapp",
				util.VMNameLabel: "vm1", // Plugin adds label
			},
			expectedAfterRestore: map[string]string{
				"app": "myapp", // Only original labels remain
			},
			expectedAnnsAfterBackup: map[string]string{
				"description":                   "test pvc",
				util.VMNameLabelAddedAnnotation: "true",
			},
			expectedAnnsAfterRestore: map[string]string{
				"description": "test pvc", // Only original annotations remain
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create copies for backup and restore phases
			backupPVC := tc.originalPVC.DeepCopy()

			// Simulate backup phase
			backupAction := NewPVCBackupItemAction(logrus.New())
			backupAction.addVMLabelsToBackupContent(backupPVC, tc.discoveredVMs)

			// Verify backup results
			assert.Equal(t, tc.expectedAfterBackup, backupPVC.Labels, "Labels after backup")
			assert.Equal(t, tc.expectedAnnsAfterBackup, backupPVC.Annotations, "Annotations after backup")

			// Simulate restore phase (using the backup result)
			restorePVC := backupPVC.DeepCopy()
			restoreAction := NewPVCRestoreItemAction(logrus.New())
			restoreAction.removeVMLabelsFromRestoreContent(restorePVC)

			// Verify restore results
			assert.Equal(t, tc.expectedAfterRestore, restorePVC.Labels, "Labels after restore")
			assert.Equal(t, tc.expectedAnnsAfterRestore, restorePVC.Annotations, "Annotations after restore")
		})
	}
}

func TestLabelCollisionScenarios(t *testing.T) {
	testCases := []struct {
		name                    string
		originalPVC            *corev1api.PersistentVolumeClaim
		discoveredVMs          []string
		expectedAfterBackup    map[string]string
		expectedAfterRestore   map[string]string
		expectedAnnsAfterBackup map[string]string
		expectedAnnsAfterRestore map[string]string
	}{
		{
			name: "User has vm-name label - preservation and restore cycle",
			originalPVC: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "user-original-vm", // User's original value
						"app":            "myapp",
					},
					Annotations: map[string]string{
						"description": "test pvc",
					},
				},
			},
			discoveredVMs: []string{"plugin-discovered-vm"},
			expectedAfterBackup: map[string]string{
				"app":            "myapp",
				util.VMNameLabel: "plugin-discovered-vm", // Plugin overwrites but preserves original
			},
			expectedAfterRestore: map[string]string{
				"app":            "myapp",
				util.VMNameLabel: "user-original-vm", // RESTORED TO EXACT ORIGINAL!
			},
			expectedAnnsAfterBackup: map[string]string{
				"description":                     "test pvc",
				util.VMNameLabelAddedAnnotation:   "true",
				util.VMNameOriginalAnnotation:     "user-original-vm", // Original preserved
			},
			expectedAnnsAfterRestore: map[string]string{
				"description": "test pvc", // Back to original - no plugin annotations
			},
		},
		{
			name: "Plugin-managed labels properly removed in restore",
			originalPVC: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app": "myapp", // Only user labels
					},
					Annotations: map[string]string{
						"description": "test pvc",
					},
				},
			},
			discoveredVMs: []string{"vm1"}, // Plugin discovers VM
			expectedAfterBackup: map[string]string{
				"app":            "myapp",
				util.VMNameLabel: "vm1", // Plugin adds label
			},
			expectedAfterRestore: map[string]string{
				"app": "myapp", // Only original labels remain
			},
			expectedAnnsAfterBackup: map[string]string{
				"description":                   "test pvc",
				util.VMNameLabelAddedAnnotation: "true",
			},
			expectedAnnsAfterRestore: map[string]string{
				"description": "test pvc", // Only original annotations remain
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create copies for backup and restore phases
			backupPVC := tc.originalPVC.DeepCopy()

			// Simulate backup phase
			backupAction := NewPVCBackupItemAction(logrus.New())
			backupAction.addVMLabelsToBackupContent(backupPVC, tc.discoveredVMs)

			// Verify backup results
			assert.Equal(t, tc.expectedAfterBackup, backupPVC.Labels, "Labels after backup")
			assert.Equal(t, tc.expectedAnnsAfterBackup, backupPVC.Annotations, "Annotations after backup")

			// Simulate restore phase (using the backup result)
			restorePVC := backupPVC.DeepCopy()
			restoreAction := NewPVCRestoreItemAction(logrus.New())
			restoreAction.removeVMLabelsFromRestoreContent(restorePVC)

			// Verify restore results
			assert.Equal(t, tc.expectedAfterRestore, restorePVC.Labels, "Labels after restore")
			assert.Equal(t, tc.expectedAnnsAfterRestore, restorePVC.Annotations, "Annotations after restore")
		})
	}
}

func TestRemoveVMLabelsFromRestoreContent(t *testing.T) {
	testCases := []struct {
		name            string
		pvc             *corev1api.PersistentVolumeClaim
		expectedLabels  map[string]string
		expectedAnns    map[string]string
	}{
		{
			name: "Remove VM labels when plugin added them",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "vm1",
						"app":            "myapp",
					},
					Annotations: map[string]string{
						util.VMNameLabelAddedAnnotation: "true",
						"description":                   "test pvc",
					},
				},
			},
			expectedLabels: map[string]string{
				"app": "myapp",
			},
			expectedAnns: map[string]string{
				"description": "test pvc",
			},
		},
		{
			name: "Don't remove anything when no plugin annotations present",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "vm1",
						"app":            "myapp",
					},
					Annotations: map[string]string{
						"description": "test pvc",
					},
				},
			},
			expectedLabels: map[string]string{
				util.VMNameLabel: "vm1",
				"app":            "myapp",
			},
			expectedAnns: map[string]string{
				"description": "test pvc",
			},
		},
		{
			name: "Restore original user value when preserved",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "plugin-vm",
						"app":            "myapp",
					},
					Annotations: map[string]string{
						util.VMNameLabelAddedAnnotation: "true",
						util.VMNameOriginalAnnotation:   "user-original-vm",
						"description":                   "test pvc",
					},
				},
			},
			expectedLabels: map[string]string{
				util.VMNameLabel: "user-original-vm", // Restored to original
				"app":            "myapp",
			},
			expectedAnns: map[string]string{
				"description": "test pvc",
			},
		},
		{
			name: "Handle case where user had similar annotation name",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "plugin-vm",
						"app":            "myapp",
					},
					Annotations: map[string]string{
						util.VMNameLabelAddedAnnotation:                   "true",
						util.VMNameOriginalAnnotation:                     "user-original-vm", // Plugin's preservation
						"velero.kubevirt.io/vm-name-original":             "user-annotation",  // User's similar annotation
						"velero.kubevirt-velero-plugin.io/user-metadata": "user-data",        // User's annotation with similar prefix
						"description": "test pvc",
					},
				},
			},
			expectedLabels: map[string]string{
				util.VMNameLabel: "user-original-vm", // Restored from plugin's annotation
				"app":            "myapp",
			},
			expectedAnns: map[string]string{
				"velero.kubevirt.io/vm-name-original":             "user-annotation", // User's annotation preserved
				"velero.kubevirt-velero-plugin.io/user-metadata": "user-data",       // User's annotation preserved
				"description": "test pvc",
			},
		},
		{
			name: "Handle user annotation that looks like tracking annotation",
			pvc: &corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels: map[string]string{
						util.VMNameLabel: "plugin-vm",
						"app":            "myapp",
					},
					Annotations: map[string]string{
						util.VMNameLabelAddedAnnotation:       "true",                    // Plugin's tracking
						util.VMNameOriginalAnnotation:         "user-original-vm",       // Plugin's preservation
						"velero.kubevirt.io/vm-label-added":   "user-value",             // User's similar annotation
						"kubevirt-velero-plugin.io/tracking": "user-tracking",          // User's similar annotation
						"description":                         "test pvc",
					},
				},
			},
			expectedLabels: map[string]string{
				util.VMNameLabel: "user-original-vm", // Restored from plugin's preservation annotation
				"app":            "myapp",
			},
			expectedAnns: map[string]string{
				"velero.kubevirt.io/vm-label-added":   "user-value",     // User's annotation preserved
				"kubevirt-velero-plugin.io/tracking": "user-tracking",  // User's annotation preserved
				"description":                         "test pvc",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			action := NewPVCRestoreItemAction(logrus.New())
			action.removeVMLabelsFromRestoreContent(tc.pvc)

			assert.Equal(t, tc.expectedLabels, tc.pvc.Labels)
			assert.Equal(t, tc.expectedAnns, tc.pvc.Annotations)
		})
	}
}
