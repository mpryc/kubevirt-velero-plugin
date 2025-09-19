package plugin

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumesnapshot/v1"
)

func TestVolumeSnapshotBackupExecute(t *testing.T) {
	testCases := []struct {
		name           string
		volumeSnapshot *snapshotv1.VolumeSnapshot
		expectedLabels map[string]string
		expectSkip     bool
	}{
		{
			"VolumeSnapshot without PVC source should be skipped",
			&snapshotv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vs",
					Namespace: "test-namespace",
					UID:       "vs-uid-123",
				},
				Spec: snapshotv1.VolumeSnapshotSpec{
					Source: snapshotv1.VolumeSnapshotSource{
						// No PersistentVolumeClaimName set
					},
				},
			},
			map[string]string{},
			true,
		},
	}

	logger := logrus.StandardLogger()
	action := NewVolumeSnapshotBackupItemAction(logger)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert to unstructured
			item, err := runtime.DefaultUnstructuredConverter.ToUnstructured(tc.volumeSnapshot)
			if !assert.NoError(t, err) {
				return
			}

			backup := &v1.Backup{}
			result, _, err := action.Execute(&unstructured.Unstructured{Object: item}, backup)

			if tc.expectSkip {
				// For cases where we skip processing, just verify no error
				assert.NoError(t, err)
				assert.NotNil(t, result)
				return
			}

			if !assert.NoError(t, err) {
				return
			}

			// Extract the result VolumeSnapshot
			var resultVS snapshotv1.VolumeSnapshot
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(result.UnstructuredContent(), &resultVS)
			if !assert.NoError(t, err) {
				return
			}

			// Verify expected labels are present
			for expectedKey, expectedValue := range tc.expectedLabels {
				actualValue, exists := resultVS.Labels[expectedKey]
				assert.True(t, exists, "Expected label %s not found", expectedKey)
				assert.Equal(t, expectedValue, actualValue, "Label %s value mismatch", expectedKey)
			}
		})
	}
}