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

	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"kubevirt.io/kubevirt-velero-plugin/pkg/util"
)

// PVCBackupItemAction is a backup item action for adding resource name labels to PVCs
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
		IncludedResources: []string{"PersistentVolumeClaim"},
	}, nil
}

// Execute adds resource name labels to PVCs for selective restore
func (p *PVCBackupItemAction) Execute(item runtime.Unstructured, backup *v1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.log.Info("Executing PVCBackupItemAction")

	if backup == nil {
		return nil, nil, fmt.Errorf("backup object nil!")
	}

	var pvc corev1api.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &pvc); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	p.log.Infof("handling PVC %v/%v", pvc.GetNamespace(), pvc.GetName())

	// Add PVC UID label for selective restore
	if err := p.addResourceUIDLabel(&pvc); err != nil {
		return nil, nil, err
	}

	// Convert back to unstructured
	unstructuredPVC, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	extra := []velero.ResourceIdentifier{}
	return &unstructured.Unstructured{Object: unstructuredPVC}, extra, nil
}

// addResourceUIDLabel adds the PVC UID as a label for selective restore
// Implements collision detection and preservation logic per design document
func (p *PVCBackupItemAction) addResourceUIDLabel(pvc *corev1api.PersistentVolumeClaim) error {
	if pvc.Labels == nil {
		pvc.Labels = make(map[string]string)
	}

	if pvc.Annotations == nil {
		pvc.Annotations = make(map[string]string)
	}

	// Get PVC UID - no truncation needed as UIDs are always valid label values
	pvcUID := string(pvc.GetUID())
	if pvcUID == "" {
		p.log.Warnf("PVC %s/%s has empty UID, skipping UID labeling", pvc.GetNamespace(), pvc.GetName())
		return nil
	}

	// Check if label already exists
	if existingValue, exists := pvc.Labels[util.PVCUIDLabel]; exists {
		if existingValue != pvcUID {
			// Label exists with different value - preserve original value
			pvc.Annotations[util.OriginalPVCUIDAnnotation] = existingValue
			p.log.Infof("Preserving original label value %s=%s for PVC %s/%s", util.PVCUIDLabel, existingValue, pvc.GetNamespace(), pvc.GetName())
		}
		// If existing value matches PVC UID, no action needed
	}

	// Set/overwrite the label with PVC UID
	pvc.Labels[util.PVCUIDLabel] = pvcUID
	p.log.Infof("Added resource UID label %s=%s to PVC %s/%s", util.PVCUIDLabel, pvcUID, pvc.GetNamespace(), pvc.GetName())

	return nil
}
