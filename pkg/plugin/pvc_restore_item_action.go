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
 * Copyright 2022 Red Hat, Inc.
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


// PVCRestoreItemAction is a backup item action for restoring DataVolumes
type PVCRestoreItemAction struct {
	log logrus.FieldLogger
}

// NewPVCRestoreItemAction instantiates a PVCRestoreItemAction.
func NewPVCRestoreItemAction(log logrus.FieldLogger) *PVCRestoreItemAction {
	return &PVCRestoreItemAction{log: log}
}

// AppliesTo returns information about which resources this action should be invoked for.
func (p *PVCRestoreItemAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
			IncludedResources: []string{"PersistentVolumeClaim"},
		},
		nil
}

// Execute if the PVC and the corresponding DV is not SUCCESSFULL - then skip PVC
func (p *PVCRestoreItemAction) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	p.log.Info("Executing PVCRestoreItemAction")
	// TODO: Remove this extensive logging after debugging
	p.log.Infof("DEBUG: PVCRestoreItemAction Execute method started")

	if input == nil {
		p.log.Info("DEBUG: input object is nil, returning error")
		return nil, fmt.Errorf("input object nil!")
	}
	p.log.Info("DEBUG: input object is not nil")

	var pvc corev1api.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &pvc); err != nil {
		p.log.Infof("DEBUG: failed to convert unstructured item to PVC: %v", err)
		return nil, errors.WithStack(err)
	}
	p.log.Infof("DEBUG: successfully converted unstructured item to PVC")

	p.log.Infof("handling PVC %v/%v", pvc.GetNamespace(), pvc.GetName())
	// TODO: Remove this extensive logging after debugging
	p.log.Infof("DEBUG: PVC details - Name: %s, Namespace: %s, UID: %s", pvc.GetName(), pvc.GetNamespace(), pvc.GetUID())
	p.log.Infof("DEBUG: PVC labels before processing: %v", pvc.Labels)
	p.log.Infof("DEBUG: PVC annotations before processing: %v", pvc.Annotations)

	annotations := pvc.GetAnnotations()
	_, inProgress := annotations[AnnInProgress]
	p.log.Infof("DEBUG: checked for AnnInProgress annotation, inProgress: %t", inProgress)
	if inProgress {
		p.log.Info("DEBUG: PVC has AnnInProgress annotation, skipping restore")
		return velero.NewRestoreItemActionExecuteOutput(input.Item).WithoutRestore(), nil
	}

	// Remove resource UID labels added during backup
	if pvc.Labels != nil {
		if _, exists := pvc.Labels[util.PVCUIDLabel]; exists {
			// Check if we preserved an original value
			if pvc.Annotations != nil {
				if originalValue, hasOriginal := pvc.Annotations[util.OriginalPVCUIDAnnotation]; hasOriginal {
					// Restore the original value
					pvc.Labels[util.PVCUIDLabel] = originalValue
					delete(pvc.Annotations, util.OriginalPVCUIDAnnotation)
					p.log.Infof("Restored original label value %s=%s for PVC %s/%s", util.PVCUIDLabel, originalValue, pvc.GetNamespace(), pvc.GetName())
				} else {
					// No original value to restore - remove the plugin-added label completely
					delete(pvc.Labels, util.PVCUIDLabel)
					p.log.Infof("Removed plugin-added label %s from PVC %s/%s", util.PVCUIDLabel, pvc.GetNamespace(), pvc.GetName())
				}
			} else {
				// No annotations - remove the plugin-added label
				delete(pvc.Labels, util.PVCUIDLabel)
				p.log.Infof("Removed plugin-added label %s from PVC %s/%s", util.PVCUIDLabel, pvc.GetNamespace(), pvc.GetName())
			}
		}
	}

	// Convert back to unstructured
	p.log.Info("DEBUG: converting PVC back to unstructured")
	item, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
	if err != nil {
		p.log.Infof("DEBUG: failed to convert PVC back to unstructured: %v", err)
		return nil, errors.WithStack(err)
	}
	p.log.Info("DEBUG: successfully converted PVC back to unstructured")

	p.log.Infof("DEBUG: PVCRestoreItemAction Execute method completed successfully, returning PVC %s/%s", pvc.GetNamespace(), pvc.GetName())
	return velero.NewRestoreItemActionExecuteOutput(&unstructured.Unstructured{Object: item}), nil
}
