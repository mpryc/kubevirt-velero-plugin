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
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"kubevirt.io/kubevirt-velero-plugin/pkg/util"

	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// PVCBackupItemAction is a backup item action for backing up PersistentVolumeClaims
type PVCBackupItemAction struct {
	log logrus.FieldLogger
}

// NewPVCBackupItemAction instantiates a PVCBackupItemAction.
func NewPVCBackupItemAction(log logrus.FieldLogger) *PVCBackupItemAction {
	return &PVCBackupItemAction{log: log}
}

// AppliesTo returns information about which resources this action should be invoked for.
func (p *PVCBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
			IncludedResources: []string{
				"PersistentVolumeClaim",
			},
		},
		nil
}

// Execute allows the ItemAction to perform arbitrary logic with the item being backed up,
// in this case, adding UID labels to PVCs for selective restore functionality.
func (p *PVCBackupItemAction) Execute(item runtime.Unstructured, backup *v1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.log.Info("Executing PVCBackupItemAction")

	metadata, err := meta.Accessor(item)
	if err != nil {
		p.log.WithError(err).Error("Failed to get metadata accessor for PVC")
		return nil, nil, err
	}

	pvcName := metadata.GetName()
	pvcNamespace := metadata.GetNamespace()
	p.log.Infof("Processing PVC %s/%s", pvcNamespace, pvcName)

	// Log existing labels and annotations for debugging
	existingLabels := metadata.GetLabels()
	existingAnnotations := metadata.GetAnnotations()
	p.log.Infof("PVC %s/%s existing labels: %v", pvcNamespace, pvcName, existingLabels)
	p.log.Infof("PVC %s/%s existing annotations: %v", pvcNamespace, pvcName, existingAnnotations)

	// Add UID label for selective restore
	labels := metadata.GetLabels()
	if labels == nil {
		p.log.Infof("PVC %s/%s has no existing labels, creating new label map", pvcNamespace, pvcName)
		labels = make(map[string]string)
	}

	pvcUID := string(metadata.GetUID())
	if pvcUID == "" {
		p.log.Warnf("PVC %s/%s has empty UID, skipping UID label addition", pvcNamespace, pvcName)
		extra := []velero.ResourceIdentifier{}
		return item, extra, nil
	}

	p.log.Infof("PVC %s/%s UID: %s", pvcNamespace, pvcName, pvcUID)

	// Handle collision detection - preserve original value if different
	if existingValue, exists := labels[util.PVCUIDLabel]; exists {
		p.log.Infof("PVC %s/%s already has %s label with value: %s", pvcNamespace, pvcName, util.PVCUIDLabel, existingValue)
		if existingValue != pvcUID {
			p.log.Infof("Label collision detected for PVC %s/%s: existing=%s, current=%s", pvcNamespace, pvcName, existingValue, pvcUID)
			annotations := metadata.GetAnnotations()
			if annotations == nil {
				p.log.Infof("Creating new annotations map for PVC %s/%s collision preservation", pvcNamespace, pvcName)
				annotations = make(map[string]string)
			}
			annotations[util.OriginalPVCUIDAnnotation] = existingValue
			metadata.SetAnnotations(annotations)
			p.log.Infof("Preserving original label value %s=%s for PVC %s/%s in annotation %s", util.PVCUIDLabel, existingValue, pvcNamespace, pvcName, util.OriginalPVCUIDAnnotation)
		} else {
			p.log.Infof("PVC %s/%s already has correct UID label value, no action needed", pvcNamespace, pvcName)
		}
	} else {
		p.log.Infof("PVC %s/%s does not have %s label, will add it", pvcNamespace, pvcName, util.PVCUIDLabel)
	}

	labels[util.PVCUIDLabel] = pvcUID
	metadata.SetLabels(labels)
	p.log.Infof("Added resource UID label %s=%s to PVC %s/%s", util.PVCUIDLabel, pvcUID, pvcNamespace, pvcName)

	// Log final state for debugging
	finalLabels := metadata.GetLabels()
	finalAnnotations := metadata.GetAnnotations()
	p.log.Infof("PVC %s/%s final labels: %v", pvcNamespace, pvcName, finalLabels)
	p.log.Infof("PVC %s/%s final annotations: %v", pvcNamespace, pvcName, finalAnnotations)

	extra := []velero.ResourceIdentifier{}
	p.log.Infof("PVCBackupItemAction completed successfully for PVC %s/%s", pvcNamespace, pvcName)
	return item, extra, nil
}
