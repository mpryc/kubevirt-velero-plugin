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
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kvcore "kubevirt.io/api/core/v1"
	"kubevirt.io/kubevirt-velero-plugin/pkg/util"

	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// PVCBackupItemAction is a backup item action for backing up PersistentVolumeClaims
type PVCBackupItemAction struct {
	log logrus.FieldLogger
	// Cache: namespace -> pvcName -> []vmName
	pvcToVMsCache map[string]map[string][]string
}

// NewPVCBackupItemAction instantiates a PVCBackupItemAction.
func NewPVCBackupItemAction(log logrus.FieldLogger) *PVCBackupItemAction {
	return &PVCBackupItemAction{
		log:           log,
		pvcToVMsCache: make(map[string]map[string][]string),
	}
}

// AppliesTo returns information about which resources this action should be invoked for.
func (p *PVCBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{
			"PersistentVolumeClaim",
		},
	}, nil
}

// Execute adds VM name labels to PVC backup content for tracking purposes
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

	// Ensure namespace cache is populated
	if err := p.ensureNamespaceCachePopulated(pvc.Namespace, backup); err != nil {
		p.log.Errorf("Failed to populate cache for namespace %s: %v", pvc.Namespace, err)
		// Don't fail the backup, just proceed without labeling
	} else {
		// Get VM names from cache
		if nsCache, exists := p.pvcToVMsCache[pvc.Namespace]; exists {
			if vmNames, exists := nsCache[pvc.Name]; exists && len(vmNames) > 0 {
				p.addVMLabelsToBackupContent(&pvc, vmNames)
			}
		}
	}

	pvcMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	return &unstructured.Unstructured{Object: pvcMap}, []velero.ResourceIdentifier{}, nil
}

// ensureNamespaceCachePopulated populates the PVC->VM mapping cache for a namespace if not already done
func (p *PVCBackupItemAction) ensureNamespaceCachePopulated(namespace string, backup *v1.Backup) error {
	// Check if cache already exists for this namespace
	if _, exists := p.pvcToVMsCache[namespace]; exists {
		return nil
	}

	// Only check if VMs are included in backup
	if !util.IsResourceInBackup("virtualmachines", backup) {
		// Create empty cache for this namespace
		p.pvcToVMsCache[namespace] = make(map[string][]string)
		return nil
	}

	p.log.Infof("Populating PVC->VM cache for namespace %s", namespace)

	// Get all VMs in the namespace
	client, err := util.GetKubeVirtclient()
	if err != nil {
		return err
	}

	vms, err := (*client).VirtualMachine(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Initialize cache for this namespace
	nsCache := make(map[string][]string)

	// Build PVC->VMs mapping
	for _, vm := range vms.Items {
		pvcNames := p.getPVCsUsedByVM(&vm)
		for _, pvcName := range pvcNames {
			nsCache[pvcName] = append(nsCache[pvcName], vm.Name)
		}
	}

	p.pvcToVMsCache[namespace] = nsCache
	p.log.Infof("Cached %d PVC->VM mappings for namespace %s", len(nsCache), namespace)
	return nil
}

// getPVCsUsedByVM returns all PVC names used by a VM
func (p *PVCBackupItemAction) getPVCsUsedByVM(vm *kvcore.VirtualMachine) []string {
	var pvcNames []string

	// Check volumes in VM spec
	if vm.Spec.Template != nil {
		for _, volume := range vm.Spec.Template.Spec.Volumes {
			if volume.VolumeSource.PersistentVolumeClaim != nil {
				pvcNames = append(pvcNames, volume.VolumeSource.PersistentVolumeClaim.ClaimName)
			}
			if volume.VolumeSource.DataVolume != nil {
				pvcNames = append(pvcNames, volume.VolumeSource.DataVolume.Name)
			}
		}
	}

	// Check DataVolumeTemplates
	for _, dvTemplate := range vm.Spec.DataVolumeTemplates {
		pvcNames = append(pvcNames, dvTemplate.Name)
	}

	return pvcNames
}

// addVMLabelsToBackupContent adds VM name label to the PVC backup content
// Only modifies labels that are managed by this plugin (have tracking annotations)
func (p *PVCBackupItemAction) addVMLabelsToBackupContent(pvc *corev1api.PersistentVolumeClaim, vmNames []string) {
	if len(vmNames) != 1 {
		p.log.Warnf("PVC %s/%s is used by %d VMs, but only single VM per PVC is supported. Skipping labeling.", pvc.Namespace, pvc.Name, len(vmNames))
		return
	}

	if pvc.Labels == nil {
		pvc.Labels = make(map[string]string)
	}
	if pvc.Annotations == nil {
		pvc.Annotations = make(map[string]string)
	}

	vmName := vmNames[0]

	// Check if VM label already exists
	existingLabel, labelExists := pvc.Labels[util.VMNameLabel]
	trackingAnnotation, annotationExists := pvc.Annotations[util.VMNameLabelAddedAnnotation]

	// Always add/update VM label, preserving original user value when needed
	if !labelExists {
		// Label doesn't exist - add it
		pvc.Labels[util.VMNameLabel] = vmName
		pvc.Annotations[util.VMNameLabelAddedAnnotation] = "true"
		p.log.Infof("Added VM name label to PVC %s/%s for VM: %s", pvc.Namespace, pvc.Name, vmName)
	} else if annotationExists && trackingAnnotation == "true" {
		// Label exists and is plugin-managed - update it
		pvc.Labels[util.VMNameLabel] = vmName
		p.log.Infof("Updated plugin-managed VM name label to PVC %s/%s for VM: %s", pvc.Namespace, pvc.Name, vmName)
	} else if existingLabel != vmName {
		// Label exists but is user-managed and differs - preserve original then overwrite
		pvc.Annotations[util.VMNameOriginalAnnotation] = existingLabel
		pvc.Labels[util.VMNameLabel] = vmName
		pvc.Annotations[util.VMNameLabelAddedAnnotation] = "true"
		p.log.Infof("Preserved original vm-name label value '%s' and set to discovered VM '%s' for PVC %s/%s", existingLabel, vmName, pvc.Namespace, pvc.Name)
	} else {
		// Label exists, is user-managed, and matches discovered VM - track it
		pvc.Annotations[util.VMNameLabelAddedAnnotation] = "true"
		p.log.Infof("VM name label on PVC %s/%s matches discovered VM '%s' - now tracking", pvc.Namespace, pvc.Name, vmName)
	}
}
