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
	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"kubevirt.io/kubevirt-velero-plugin/pkg/util"
)

func TestPVCBackupItemActionExecute(t *testing.T) {
	logrus.SetLevel(logrus.ErrorLevel)
	action := NewPVCBackupItemAction(logrus.StandardLogger())
	backup := &v1.Backup{}

	t.Run("Add UID label to PVC without existing labels", func(t *testing.T) {
		testUID := "633ab84c-8529-487c-8848-99b40fbda9f5"
		input := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "PersistentVolumeClaim",
				"metadata": map[string]interface{}{
					"name":      "test-pvc",
					"namespace": "test-namespace",
					"uid":       testUID,
				},
				"spec": map[string]interface{}{},
			},
		}

		result, _, err := action.Execute(input, backup)
		if !assert.NoError(t, err) {
			return
		}

		var resultPVC corev1api.PersistentVolumeClaim
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(result.UnstructuredContent(), &resultPVC)
		if !assert.NoError(t, err) {
			return
		}

		// Verify PVC UID label was added
		actualValue, exists := resultPVC.Labels[util.PVCUIDLabel]
		assert.True(t, exists, "Expected label %s not found", util.PVCUIDLabel)
		assert.Equal(t, testUID, actualValue, "Label %s value mismatch", util.PVCUIDLabel)

		// Verify no collision annotation was added
		_, hasCollisionAnnotation := resultPVC.Annotations[util.OriginalPVCUIDAnnotation]
		assert.False(t, hasCollisionAnnotation, "Should not have collision annotation when no collision")
	})

	t.Run("Handle UID label collision - preserve original value", func(t *testing.T) {
		testUID := "633ab84c-8529-487c-8848-99b40fbda9f5"
		userUID := "user-defined-uid-value"
		input := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "PersistentVolumeClaim",
				"metadata": map[string]interface{}{
					"name":      "data-pvc",
					"namespace": "test-namespace",
					"uid":       testUID,
					"labels": map[string]interface{}{
						util.PVCUIDLabel: userUID, // User accidentally used same label key
					},
				},
				"spec": map[string]interface{}{},
			},
		}

		result, _, err := action.Execute(input, backup)
		if !assert.NoError(t, err) {
			return
		}

		var resultPVC corev1api.PersistentVolumeClaim
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(result.UnstructuredContent(), &resultPVC)
		if !assert.NoError(t, err) {
			return
		}

		// Verify label was overwritten with actual PVC UID
		actualValue, exists := resultPVC.Labels[util.PVCUIDLabel]
		assert.True(t, exists, "Expected label %s not found", util.PVCUIDLabel)
		assert.Equal(t, testUID, actualValue, "Label %s should be actual PVC UID", util.PVCUIDLabel)

		// Verify original user value was preserved in annotation
		originalValue, hasOriginal := resultPVC.Annotations[util.OriginalPVCUIDAnnotation]
		assert.True(t, hasOriginal, "Original value should be preserved in annotation")
		assert.Equal(t, userUID, originalValue, "Original value mismatch")
	})

	t.Run("Handle UID label that matches PVC UID - no collision", func(t *testing.T) {
		testUID := "789def01-2345-6789-abcd-ef0123456789"
		input := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "PersistentVolumeClaim",
				"metadata": map[string]interface{}{
					"name":      "exact-match-pvc",
					"namespace": "test-namespace",
					"uid":       testUID,
					"labels": map[string]interface{}{
						util.PVCUIDLabel: testUID, // Matches actual PVC UID
					},
				},
				"spec": map[string]interface{}{},
			},
		}

		result, _, err := action.Execute(input, backup)
		if !assert.NoError(t, err) {
			return
		}

		var resultPVC corev1api.PersistentVolumeClaim
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(result.UnstructuredContent(), &resultPVC)
		if !assert.NoError(t, err) {
			return
		}

		// Verify label remains the same
		actualValue, exists := resultPVC.Labels[util.PVCUIDLabel]
		assert.True(t, exists, "Expected label %s not found", util.PVCUIDLabel)
		assert.Equal(t, testUID, actualValue, "Label %s value mismatch", util.PVCUIDLabel)

		// Verify no collision annotation was added
		_, hasCollisionAnnotation := resultPVC.Annotations[util.OriginalPVCUIDAnnotation]
		assert.False(t, hasCollisionAnnotation, "Should not have collision annotation when values match")
	})
}

func TestPVCBackupItemActionAppliesTo(t *testing.T) {
	action := NewPVCBackupItemAction(logrus.StandardLogger())
	selector, err := action.AppliesTo()

	assert.NoError(t, err)
	assert.Equal(t, []string{"PersistentVolumeClaim"}, selector.IncludedResources)
}
