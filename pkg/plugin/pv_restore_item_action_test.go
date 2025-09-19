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
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"kubevirt.io/kubevirt-velero-plugin/pkg/util"
)

func TestPVRestoreItemActionExecute(t *testing.T) {
	testCases := []struct {
		name           string
		input          velero.RestoreItemActionExecuteInput
		expectedLabels map[string]string
	}{
		{
			"Remove resource UID label from PV",
			velero.RestoreItemActionExecuteInput{
				Item: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "PersistentVolume",
						"metadata": map[string]interface{}{
							"name": "test-pv",
							"uid":  "789def01-2345-6789-abcd-ef0123456789",
							"labels": map[string]interface{}{
								util.PVUIDLabel: "789def01-2345-6789-abcd-ef0123456789",
								"other-label":   "other-value",
							},
						},
						"spec": map[string]interface{}{},
					},
				},
			},
			map[string]string{
				"other-label": "other-value",
			},
		},
		{
			"Restore original UID label value from collision annotation",
			velero.RestoreItemActionExecuteInput{
				Item: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "PersistentVolume",
						"metadata": map[string]interface{}{
							"name": "collision-pv",
							"uid":  "789def01-2345-6789-abcd-ef0123456789",
							"labels": map[string]interface{}{
								util.PVUIDLabel: "789def01-2345-6789-abcd-ef0123456789", // Plugin-added during backup
								"other-label":   "other-value",
							},
							"annotations": map[string]interface{}{
								util.OriginalPVUIDAnnotation: "original-user-pv-uid-value", // User's original value
							},
						},
						"spec": map[string]interface{}{},
					},
				},
			},
			map[string]string{
				util.PVUIDLabel: "original-user-pv-uid-value", // Should be restored to original
				"other-label":   "other-value",
			},
		},
		{
			"Handle PV without resource UID label",
			velero.RestoreItemActionExecuteInput{
				Item: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "PersistentVolume",
						"metadata": map[string]interface{}{
							"name": "test-pv",
							"uid":  "456ghi78-9012-3456-7890-abcdef123456",
							"labels": map[string]interface{}{
								"existing-label": "existing-value",
							},
						},
						"spec": map[string]interface{}{},
					},
				},
			},
			map[string]string{
				"existing-label": "existing-value",
			},
		},
		{
			"Handle PV without any labels",
			velero.RestoreItemActionExecuteInput{
				Item: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "PersistentVolume",
						"metadata": map[string]interface{}{
							"name": "test-pv",
						},
						"spec": map[string]interface{}{},
					},
				},
			},
			map[string]string{},
		},
	}

	logrus.SetLevel(logrus.ErrorLevel)
	action := NewPVRestoreItemAction(logrus.StandardLogger())

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := action.Execute(&tc.input)
			if !assert.NoError(t, err) {
				return
			}

			assert.False(t, result.SkipRestore)

			// Extract the result PV
			var resultPV corev1api.PersistentVolume
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(result.UpdatedItem.UnstructuredContent(), &resultPV)
			if !assert.NoError(t, err) {
				return
			}

			// Verify expected labels are present and resource name label is removed
			if tc.expectedLabels == nil {
				tc.expectedLabels = make(map[string]string)
			}

			if resultPV.Labels == nil {
				resultPV.Labels = make(map[string]string)
			}

			assert.Equal(t, len(tc.expectedLabels), len(resultPV.Labels), "Unexpected number of labels")

			for expectedKey, expectedValue := range tc.expectedLabels {
				actualValue, exists := resultPV.Labels[expectedKey]
				assert.True(t, exists, "Expected label %s not found", expectedKey)
				assert.Equal(t, expectedValue, actualValue, "Label %s value mismatch", expectedKey)
			}

			// Verify resource UID label was removed (unless it was restored to original)
			if tc.expectedLabels[util.PVUIDLabel] == "" {
				_, exists := resultPV.Labels[util.PVUIDLabel]
				assert.False(t, exists, "Resource UID label should have been removed")
			}

			// Verify collision annotation was removed if it existed
			_, hasCollisionAnnotation := resultPV.Annotations[util.OriginalPVUIDAnnotation]
			assert.False(t, hasCollisionAnnotation, "Collision annotation should have been removed")
		})
	}
}

func TestPVRestoreItemActionAppliesTo(t *testing.T) {
	action := NewPVRestoreItemAction(logrus.StandardLogger())
	selector, err := action.AppliesTo()

	assert.NoError(t, err)
	assert.Equal(t, []string{"PersistentVolume"}, selector.IncludedResources)
}
