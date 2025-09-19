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

// PVBackupItemAction is a backup item action for adding resource name labels to PVs
type PVBackupItemAction struct {
	log logrus.FieldLogger
}

// NewPVBackupItemAction instantiates a PVBackupItemAction.
func NewPVBackupItemAction(log logrus.FieldLogger) *PVBackupItemAction {
	return &PVBackupItemAction{log: log}
}

// AppliesTo returns information about which resources this action should be invoked for.
func (p *PVBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"PersistentVolume"},
	}, nil
}

// Execute adds resource name labels to PVs for selective restore
func (p *PVBackupItemAction) Execute(item runtime.Unstructured, backup *v1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.log.Info("Executing PVBackupItemAction")

	if backup == nil {
		return nil, nil, fmt.Errorf("backup object nil!")
	}

	var pv corev1api.PersistentVolume
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &pv); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	p.log.Infof("handling PV %v", pv.GetName())

	// Add PV UID label for selective restore
	if err := p.addResourceUIDLabel(&pv); err != nil {
		return nil, nil, err
	}

	// Convert back to unstructured
	unstructuredPV, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pv)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	extra := []velero.ResourceIdentifier{}
	return &unstructured.Unstructured{Object: unstructuredPV}, extra, nil
}

// addResourceUIDLabel adds the PV UID as a label for selective restore
// Implements collision detection and preservation logic per design document
func (p *PVBackupItemAction) addResourceUIDLabel(pv *corev1api.PersistentVolume) error {
	if pv.Labels == nil {
		pv.Labels = make(map[string]string)
	}

	if pv.Annotations == nil {
		pv.Annotations = make(map[string]string)
	}

	// Get PV UID - no truncation needed as UIDs are always valid label values
	pvUID := string(pv.GetUID())
	if pvUID == "" {
		p.log.Warnf("PV %s has empty UID, skipping UID labeling", pv.GetName())
		return nil
	}

	// Check if label already exists
	if existingValue, exists := pv.Labels[util.PVUIDLabel]; exists {
		if existingValue != pvUID {
			// Label exists with different value - preserve original value
			pv.Annotations[util.OriginalPVUIDAnnotation] = existingValue
			p.log.Infof("Preserving original label value %s=%s for PV %s", util.PVUIDLabel, existingValue, pv.GetName())
		}
		// If existing value matches PV UID, no action needed
	}

	// Set/overwrite the label with PV UID
	pv.Labels[util.PVUIDLabel] = pvUID
	p.log.Infof("Added resource UID label %s=%s to PV %s", util.PVUIDLabel, pvUID, pv.GetName())

	return nil
}
