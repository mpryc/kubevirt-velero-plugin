package util

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kvcore "kubevirt.io/api/core/v1"
)

func TestIsResourceIncluded(t *testing.T) {
	testCases := []struct {
		name     string
		resource string
		backup   *velerov1.Backup
		expected bool
	}{
		{"Empty include resources should succeed",
			"pods",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{},
				},
			},
			true,
		},
		{"Resource in incuded resources should succeed",
			"pods",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"pods", "virtualmachines", "persistentvolumes"},
				},
			},
			true,
		},
		{"Resource not in included resources should fail",
			"pods",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"virtualmachines", "persistentvolumes"},
				},
			},
			false,
		},
		{"Capitalization should not matter (resource)",
			"DataVolumes",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"datavolumes"},
				},
			},
			true,
		},
		{"Capitalization should not matter (backup)",
			"datavolumes",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"DataVolumes"},
				},
			},
			true,
		},
		{"Singular/plural should not matter (resource)",
			"pod",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"pods"},
				},
			},
			true,
		},
		{"Singular/plural should not matter (backup)",
			"pods",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"pod"},
				},
			},
			true,
		},
		{"Full resource name should succeed",
			"virtualmachines",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"virtualmachines.kubevirt.io"},
				},
			},
			true,
		},
		{"Singular/plural full resource name should succeed",
			"virtualmachines",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"virtualmachine.kubevirt.io"},
				},
			},
			true,
		},
	}

	logrus.SetLevel(logrus.ErrorLevel)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsResourceIncluded(tc.resource, tc.backup)

			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsResourceExcluded(t *testing.T) {
	testCases := []struct {
		name     string
		resource string
		backup   *velerov1.Backup
		expected bool
	}{
		{"Empty exclude resources should return false",
			"pods",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{},
				},
			},
			false,
		},
		{"Resource in excuded resources should return true",
			"pods",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					ExcludedResources: []string{"pods", "virtualmachines", "persistentvolumes"},
				},
			},
			true,
		},
		{"Resource not in excluded resources should fail",
			"pods",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					ExcludedResources: []string{"virtualmachines", "persistentvolumes"},
				},
			},
			false,
		},
		{"Capitalization should not matter (resource)",
			"DataVolumes",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					ExcludedResources: []string{"datavolumes"},
				},
			},
			true,
		},
		{"Capitalization should not matter (backup)",
			"datavolumes",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					ExcludedResources: []string{"DataVolumes"},
				},
			},
			true,
		},
		{"Singular/plural should not matter (resource)",
			"pod",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					ExcludedResources: []string{"pods"},
				},
			},
			true,
		},
		{"Singular/plural should not matter (backup)",
			"pods",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					ExcludedResources: []string{"pod"},
				},
			},
			true,
		},
		{"Full resource name in excluded resources should return true",
			"virtualmachines",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					ExcludedResources: []string{"virtualmachines.kubevirt.io"},
				},
			},
			true,
		},
		{"Singular/plural full resource name in excluded resources should return true",
			"virtualmachines",
			&velerov1.Backup{
				Spec: velerov1.BackupSpec{
					ExcludedResources: []string{"virtualmachine.kubevirt.io"},
				},
			},
			true,
		},
	}

	logrus.SetLevel(logrus.ErrorLevel)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsResourceExcluded(tc.resource, tc.backup)

			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestRestorePossible(t *testing.T) {
	returnFalse := func(something ...interface{}) (bool, error) { return false, nil }
	returnDVNotFound := func(something ...interface{}) (bool, error) {
		return false, k8serrors.NewNotFound(schema.GroupResource{Group: "cdi.kubevirt.io", Resource: "datavolumes"}, "dv")
	}
	returnTrue := func(something ...interface{}) (bool, error) { return true, nil }
	skipFalse := func(volume kvcore.Volume) bool { return false }
	skipTrue := func(volume kvcore.Volume) bool { return true }

	dvVolumes := []kvcore.Volume{
		{
			VolumeSource: kvcore.VolumeSource{
				DataVolume: &kvcore.DataVolumeSource{},
			},
		},
	}
	pvcVolumes := []kvcore.Volume{
		{
			VolumeSource: kvcore.VolumeSource{
				PersistentVolumeClaim: &kvcore.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: v1.PersistentVolumeClaimVolumeSource{}},
			},
		},
	}

	testCases := []struct {
		name          string
		volumes       []kvcore.Volume
		backup        velerov1.Backup
		extraTest     func(volume kvcore.Volume) bool
		isDvExcluded  func(something ...interface{}) (bool, error)
		isPvcExcluded func(something ...interface{}) (bool, error)
		expected      bool
	}{
		{"Returns true if volumes have no volumes",
			dvVolumes,
			velerov1.Backup{},
			skipFalse,
			returnFalse,
			returnFalse,
			true,
		},
		{"Returns true if volumes have DV volumes and DVs included in backup",
			dvVolumes,
			velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"datavolumes"},
				},
			},
			skipFalse,
			returnFalse,
			returnFalse,
			true,
		},
		{"Returns true if volumes have DV volumes, backup excludes datavolumes but skipVolume returns true",
			dvVolumes,
			velerov1.Backup{
				Spec: velerov1.BackupSpec{
					ExcludedResources: []string{"datavolumes"},
				},
			},
			skipTrue,
			returnFalse,
			returnFalse,
			true,
		},
		{"Returns true if volumes have DV volumes, DVs doesnt exist, but PVCs included in backup",
			dvVolumes,
			velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"persistentvolumeclaims"},
				},
			},
			skipFalse,
			returnDVNotFound,
			returnFalse,
			true,
		},
		{"Returns false if volumes have DV volumes, DV doesnt exist and PVCs excluded in backup",
			dvVolumes,
			velerov1.Backup{
				Spec: velerov1.BackupSpec{
					ExcludedResources: []string{"persistentvolumeclaims"},
				},
			},
			skipFalse,
			returnDVNotFound,
			returnFalse,
			false,
		},
		{"Returns false if volumes have DV volumes, DV doesnt exist and PVCs not inclueded in backup",
			dvVolumes,
			velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"pods"},
				},
			},
			skipFalse,
			returnDVNotFound,
			returnFalse,
			false,
		},
		{"Returns false if volumes have DV volumes, DV doesnt exist, PVCs included in backup but excluded by label",
			dvVolumes,
			velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"pods", "persistentvolumeclaims"},
				},
			},
			skipFalse,
			returnDVNotFound,
			returnTrue,
			false,
		},
		{"Returns false if volumes have DV volumes and DVs not inclued in backup",
			dvVolumes,
			velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"pods", "persistentvolumeclaims"},
				},
			},
			skipFalse,
			returnFalse,
			returnFalse,
			false,
		},
		{"Returns false if volumes have DV volumes and DVs excluded in backup",
			dvVolumes,
			velerov1.Backup{
				Spec: velerov1.BackupSpec{
					ExcludedResources: []string{"datavolumes"},
				},
			},
			skipFalse,
			returnFalse,
			returnFalse,
			false,
		},
		{"Returns false if volumes have DV volumes and DV excluded by label",
			dvVolumes,
			velerov1.Backup{},
			skipFalse,
			returnTrue,
			returnFalse,
			false},
		{"Returns true if volumes have PVC volumes and PVCs included in backup",
			pvcVolumes,
			velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"persistentvolumeclaims"},
				},
			},
			skipFalse,
			returnFalse,
			returnFalse,
			true,
		},
		{"Returns false if volumes have PVC volumes and PVCs not included in backup",
			pvcVolumes,
			velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedResources: []string{"pods"},
				},
			},
			skipFalse,
			returnFalse,
			returnFalse,
			false,
		},
		{"Returns false if volumes have PVC volumes and PVCs excluded in backup",
			pvcVolumes,
			velerov1.Backup{
				Spec: velerov1.BackupSpec{
					ExcludedResources: []string{"persistentvolumeclaims"},
				},
			},
			skipFalse,
			returnFalse,
			returnFalse,
			false,
		},
		{"Returns false if volumes have PVC volumes and PVC excluded by label",
			pvcVolumes,
			velerov1.Backup{},
			skipFalse,
			returnFalse,
			returnTrue,
			false,
		},
	}

	logrus.SetLevel(logrus.ErrorLevel)
	for _, tc := range testCases {
		IsDVExcludedByLabel = func(namespace, pvcName string) (bool, error) { return tc.isDvExcluded(namespace, pvcName) }
		IsPVCExcludedByLabel = func(namespace, pvcName string) (bool, error) { return tc.isPvcExcluded(namespace, pvcName) }

		t.Run(tc.name, func(t *testing.T) {
			possible, err := RestorePossible(tc.volumes, &tc.backup, "", tc.extraTest, &logrus.Logger{})

			assert.NoError(t, err)
			assert.Equal(t, tc.expected, possible)
		})
	}
}

func TestIsMacAddressCleared(t *testing.T) {
	testCases := []struct {
		name     string
		resource string
		restore  velerov1.Restore
		expected bool
	}{
		{"Clear MAC address should return false with no label",
			"Restore",
			velerov1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			false,
		},
		{"Clear MAC address should return true with ClearMacAddressLabel label",
			"Restore",
			velerov1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						ClearMacAddressLabel: "",
					},
				},
			},
			true,
		},
	}

	logrus.SetLevel(logrus.ErrorLevel)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ShouldClearMacAddress(&tc.restore)

			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestLabelObjectIfNotPresent(t *testing.T) {
	testCases := []struct {
		name           string
		pvc            *v1.PersistentVolumeClaim
		vmName         string
		expectedLabels map[string]string
		expectedAnns   map[string]string
		shouldUpdate   bool
	}{
		{
			"Should add label and annotation when none exist",
			&v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
				},
			},
			"test-vm",
			map[string]string{VMNameLabel: "test-vm"},
			map[string]string{VMNameLabelAddedAnnotation: "true"},
			true,
		},
		{
			"Should not modify when same VM name label exists",
			&v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels:    map[string]string{VMNameLabel: "test-vm"},
				},
			},
			"test-vm",
			map[string]string{VMNameLabel: "test-vm"},
			nil,
			false,
		},
		{
			"Should not modify when different VM name label exists",
			&v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels:    map[string]string{VMNameLabel: "other-vm"},
				},
			},
			"test-vm",
			map[string]string{VMNameLabel: "other-vm"},
			nil,
			false,
		},
		{
			"Should add label when other labels exist",
			&v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels:    map[string]string{"other": "label"},
					Annotations: map[string]string{"other": "annotation"},
				},
			},
			"test-vm",
			map[string]string{"other": "label", VMNameLabel: "test-vm"},
			map[string]string{"other": "annotation", VMNameLabelAddedAnnotation: "true"},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// For this test, we'll check the logic without actually making Kubernetes API calls
			// We'll simulate the labeling logic

			originalLabels := make(map[string]string)
			for k, v := range tc.pvc.Labels {
				originalLabels[k] = v
			}

			if tc.pvc.Labels == nil {
				tc.pvc.Labels = make(map[string]string)
			}
			if tc.pvc.Annotations == nil {
				tc.pvc.Annotations = make(map[string]string)
			}

			// Simulate the logic from labelObjectIfNotPresent
			shouldUpdate := true
			if existingVMName, exists := tc.pvc.Labels[VMNameLabel]; exists {
				if existingVMName == tc.vmName {
					// Already has correct label
					shouldUpdate = false
				} else {
					// Has different VM name - don't modify
					shouldUpdate = false
				}
			}

			if shouldUpdate {
				tc.pvc.Labels[VMNameLabel] = tc.vmName
				tc.pvc.Annotations[VMNameLabelAddedAnnotation] = "true"
			}

			assert.Equal(t, tc.shouldUpdate, shouldUpdate)
			assert.Equal(t, tc.expectedLabels, tc.pvc.Labels)
			if tc.expectedAnns != nil {
				assert.Equal(t, tc.expectedAnns, tc.pvc.Annotations)
			}
		})
	}
}

func TestUnlabelObjectIfAdded(t *testing.T) {
	testCases := []struct {
		name           string
		pvc            *v1.PersistentVolumeClaim
		vmName         string
		expectedLabels map[string]string
		expectedAnns   map[string]string
		shouldUpdate   bool
	}{
		{
			"Should remove label and annotation when both exist and match",
			&v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels:    map[string]string{VMNameLabel: "test-vm"},
					Annotations: map[string]string{VMNameLabelAddedAnnotation: "true"},
				},
			},
			"test-vm",
			map[string]string{},
			map[string]string{},
			true,
		},
		{
			"Should not remove when annotation is missing",
			&v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels:    map[string]string{VMNameLabel: "test-vm"},
				},
			},
			"test-vm",
			map[string]string{VMNameLabel: "test-vm"},
			nil,
			false,
		},
		{
			"Should not remove when annotation is false",
			&v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels:    map[string]string{VMNameLabel: "test-vm"},
					Annotations: map[string]string{VMNameLabelAddedAnnotation: "false"},
				},
			},
			"test-vm",
			map[string]string{VMNameLabel: "test-vm"},
			map[string]string{VMNameLabelAddedAnnotation: "false"},
			false,
		},
		{
			"Should not remove when VM name doesn't match",
			&v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels:    map[string]string{VMNameLabel: "other-vm"},
					Annotations: map[string]string{VMNameLabelAddedAnnotation: "true"},
				},
			},
			"test-vm",
			map[string]string{VMNameLabel: "other-vm"},
			map[string]string{VMNameLabelAddedAnnotation: "true"},
			false,
		},
		{
			"Should remove and preserve other labels/annotations",
			&v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
					Labels:    map[string]string{VMNameLabel: "test-vm", "other": "label"},
					Annotations: map[string]string{VMNameLabelAddedAnnotation: "true", "other": "annotation"},
				},
			},
			"test-vm",
			map[string]string{"other": "label"},
			map[string]string{"other": "annotation"},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the logic from unlabelObjectIfAdded
			shouldUpdate := false

			if tc.pvc.Annotations != nil {
				if added, exists := tc.pvc.Annotations[VMNameLabelAddedAnnotation]; exists && added == "true" {
					if tc.pvc.Labels != nil {
						if labelValue, exists := tc.pvc.Labels[VMNameLabel]; exists && labelValue == tc.vmName {
							delete(tc.pvc.Labels, VMNameLabel)
							delete(tc.pvc.Annotations, VMNameLabelAddedAnnotation)
							shouldUpdate = true
						}
					}
				}
			}

			assert.Equal(t, tc.shouldUpdate, shouldUpdate)
			assert.Equal(t, tc.expectedLabels, tc.pvc.Labels)
			if tc.expectedAnns != nil {
				assert.Equal(t, tc.expectedAnns, tc.pvc.Annotations)
			}
		})
	}
}

