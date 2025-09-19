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
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"

	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"kubevirt.io/kubevirt-velero-plugin/pkg/util"
)

// PVRestoreItemAction is a restore item action for removing resource name labels from PVs
type PVRestoreItemAction struct {
	log logrus.FieldLogger
}

// NewPVRestoreItemAction instantiates a PVRestoreItemAction.
func NewPVRestoreItemAction(log logrus.FieldLogger) *PVRestoreItemAction {
	return &PVRestoreItemAction{log: log}
}

// AppliesTo returns information about which resources this action should be invoked for.
func (p *PVRestoreItemAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"PersistentVolume"},
	}, nil
}

// Execute removes resource name labels from PVs during restore
func (p *PVRestoreItemAction) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	p.log.Info("Executing PVRestoreItemAction")
	if input == nil {
		return nil, fmt.Errorf("input object nil!")
	}

	var pv corev1api.PersistentVolume
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &pv); err != nil {
		return nil, errors.WithStack(err)
	}

	p.log.Infof("handling PV %v", pv.GetName())

	// Remove resource UID labels added during backup
	p.removeResourceUIDLabels(&pv)

	// Convert back to unstructured
	item, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pv)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return velero.NewRestoreItemActionExecuteOutput(&unstructured.Unstructured{Object: item}), nil
}

// removeResourceUIDLabels removes resource UID labels added during backup
// Implements original value restoration logic per design document
func (p *PVRestoreItemAction) removeResourceUIDLabels(pv *corev1api.PersistentVolume) {
	if pv.Labels == nil {
		return
	}

	// Check if we have the plugin-added label
	if _, exists := pv.Labels[util.PVUIDLabel]; exists {
		// Check if we preserved an original value
		if pv.Annotations != nil {
			if originalValue, hasOriginal := pv.Annotations[util.OriginalPVUIDAnnotation]; hasOriginal {
				// Restore the original value
				pv.Labels[util.PVUIDLabel] = originalValue
				delete(pv.Annotations, util.OriginalPVUIDAnnotation)
				p.log.Infof("Restored original label value %s=%s for PV %s", util.PVUIDLabel, originalValue, pv.GetName())
				return
			}
		}

		// No original value to restore - remove the plugin-added label completely
		delete(pv.Labels, util.PVUIDLabel)
		p.log.Infof("Removed plugin-added label %s from PV %s", util.PVUIDLabel, pv.GetName())
	}
}
