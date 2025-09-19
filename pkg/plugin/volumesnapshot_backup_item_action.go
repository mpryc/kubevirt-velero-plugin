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

	"k8s.io/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"kubevirt.io/kubevirt-velero-plugin/pkg/util"

	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumesnapshot/v1"
)

// VolumeSnapshotBackupItemAction is a backup item action for backing up VolumeSnapshots
type VolumeSnapshotBackupItemAction struct {
	log    logrus.FieldLogger
	client kubernetes.Interface
	// Cache PVCs by namespace to avoid repeated API calls
	namespacePVCs map[string]map[string]string // namespace -> pvcName -> pvcUID
}

// NewVolumeSnapshotBackupItemAction instantiates a VolumeSnapshotBackupItemAction.
func NewVolumeSnapshotBackupItemAction(log logrus.FieldLogger) *VolumeSnapshotBackupItemAction {
	client, err := util.GetK8sClient()
	if err != nil {
		log.WithError(err).Error("Failed to get Kubernetes client")
		// Return a basic action that will handle errors during execute
		return &VolumeSnapshotBackupItemAction{
			log:           log,
			namespacePVCs: make(map[string]map[string]string),
		}
	}

	return &VolumeSnapshotBackupItemAction{
		log:           log,
		client:        client,
		namespacePVCs: make(map[string]map[string]string),
	}
}

// AppliesTo returns information about which resources this action should be invoked for.
func (p *VolumeSnapshotBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
			IncludedResources: []string{
				"VolumeSnapshot",
			},
		},
		nil
}

// Execute allows the ItemAction to perform arbitrary logic with the item being backed up,
// in this case, adding PVC UID labels to VolumeSnapshots for selective restore functionality.
func (p *VolumeSnapshotBackupItemAction) Execute(item runtime.Unstructured, backup *v1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.log.Info("Executing VolumeSnapshotBackupItemAction")

	if backup == nil {
		return nil, nil, fmt.Errorf("backup object nil!")
	}

	var volumeSnapshot snapshotv1.VolumeSnapshot
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &volumeSnapshot); err != nil {
		p.log.WithError(err).Error("Failed to convert unstructured item to VolumeSnapshot")
		return nil, nil, errors.WithStack(err)
	}

	vsName := volumeSnapshot.GetName()
	vsNamespace := volumeSnapshot.GetNamespace()
	p.log.Infof("Processing VolumeSnapshot %s/%s", vsNamespace, vsName)

	// Check if the VolumeSnapshot has a source PVC
	if volumeSnapshot.Spec.Source.PersistentVolumeClaimName == nil {
		p.log.Infof("VolumeSnapshot %s/%s does not have a PVC source, skipping UID labeling", vsNamespace, vsName)
		extra := []velero.ResourceIdentifier{}
		return item, extra, nil
	}

	pvcName := *volumeSnapshot.Spec.Source.PersistentVolumeClaimName
	p.log.Infof("VolumeSnapshot %s/%s is based on PVC %s/%s", vsNamespace, vsName, vsNamespace, pvcName)

	// Get the PVC UID efficiently using cache
	pvcUID, err := p.getPVCUID(vsNamespace, pvcName)
	if err != nil {
		p.log.WithError(err).Errorf("Failed to get PVC UID for %s/%s", vsNamespace, pvcName)
		return nil, nil, errors.WithStack(err)
	}

	if pvcUID == "" {
		p.log.Warnf("PVC %s/%s has empty UID, skipping VolumeSnapshot UID labeling", vsNamespace, pvcName)
		extra := []velero.ResourceIdentifier{}
		return item, extra, nil
	}

	p.log.Infof("PVC %s/%s UID: %s", vsNamespace, pvcName, pvcUID)

	// Log existing labels and annotations for debugging
	existingLabels := volumeSnapshot.GetLabels()
	existingAnnotations := volumeSnapshot.GetAnnotations()
	p.log.Infof("VolumeSnapshot %s/%s existing labels: %v", vsNamespace, vsName, existingLabels)
	p.log.Infof("VolumeSnapshot %s/%s existing annotations: %v", vsNamespace, vsName, existingAnnotations)

	// Add PVC UID label for selective restore
	labels := volumeSnapshot.GetLabels()
	if labels == nil {
		p.log.Infof("VolumeSnapshot %s/%s has no existing labels, creating new label map", vsNamespace, vsName)
		labels = make(map[string]string)
	}

	// Handle collision detection - preserve original value if different
	if existingValue, exists := labels[util.PVCUIDLabel]; exists {
		p.log.Infof("VolumeSnapshot %s/%s already has %s label with value: %s", vsNamespace, vsName, util.PVCUIDLabel, existingValue)
		if existingValue != pvcUID {
			p.log.Infof("Label collision detected for VolumeSnapshot %s/%s: existing=%s, pvc_uid=%s", vsNamespace, vsName, existingValue, pvcUID)
			annotations := volumeSnapshot.GetAnnotations()
			if annotations == nil {
				p.log.Infof("Creating new annotations map for VolumeSnapshot %s/%s collision preservation", vsNamespace, vsName)
				annotations = make(map[string]string)
			}
			annotations[util.OriginalVolumeSnapshotUIDAnnotation] = existingValue
			volumeSnapshot.SetAnnotations(annotations)
			p.log.Infof("Preserving original label value %s=%s for VolumeSnapshot %s/%s in annotation %s", util.PVCUIDLabel, existingValue, vsNamespace, vsName, util.OriginalVolumeSnapshotUIDAnnotation)
		} else {
			p.log.Infof("VolumeSnapshot %s/%s already has correct PVC UID label value, no action needed", vsNamespace, vsName)
		}
	} else {
		p.log.Infof("VolumeSnapshot %s/%s does not have %s label, will add it", vsNamespace, vsName, util.PVCUIDLabel)
	}

	labels[util.PVCUIDLabel] = pvcUID
	volumeSnapshot.SetLabels(labels)
	p.log.Infof("Added PVC UID label %s=%s to VolumeSnapshot %s/%s", util.PVCUIDLabel, pvcUID, vsNamespace, vsName)

	// Log final state for debugging
	finalLabels := volumeSnapshot.GetLabels()
	finalAnnotations := volumeSnapshot.GetAnnotations()
	p.log.Infof("VolumeSnapshot %s/%s final labels: %v", vsNamespace, vsName, finalLabels)
	p.log.Infof("VolumeSnapshot %s/%s final annotations: %v", vsNamespace, vsName, finalAnnotations)

	// Convert back to unstructured
	vsMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&volumeSnapshot)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	extra := []velero.ResourceIdentifier{}
	p.log.Infof("VolumeSnapshotBackupItemAction completed successfully for VolumeSnapshot %s/%s", vsNamespace, vsName)
	return &unstructured.Unstructured{Object: vsMap}, extra, nil
}

// getPVCUID efficiently retrieves the UID of a PVC using namespace-level caching
func (p *VolumeSnapshotBackupItemAction) getPVCUID(namespace, pvcName string) (string, error) {
	// Check if we have this namespace cached
	if namespacePVCs, exists := p.namespacePVCs[namespace]; exists {
		if pvcUID, found := namespacePVCs[pvcName]; found {
			return pvcUID, nil
		}
		// PVC not found in cache but namespace is cached, so it doesn't exist
		return "", fmt.Errorf("PVC %s not found in namespace %s", pvcName, namespace)
	}

	// Namespace not cached yet, fetch all PVCs in the namespace at once
	if p.client == nil {
		return "", fmt.Errorf("Kubernetes client not available")
	}

	p.log.Infof("Caching PVCs for namespace %s", namespace)
	pvcList, err := p.client.CoreV1().PersistentVolumeClaims(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", errors.WithStack(err)
	}

	// Initialize the namespace cache
	p.namespacePVCs[namespace] = make(map[string]string)

	// Cache all PVCs in this namespace
	for _, pvc := range pvcList.Items {
		p.namespacePVCs[namespace][pvc.Name] = string(pvc.UID)
	}

	p.log.Infof("Cached %d PVCs for namespace %s", len(pvcList.Items), namespace)

	// Now look up the specific PVC
	if pvcUID, found := p.namespacePVCs[namespace][pvcName]; found {
		return pvcUID, nil
	}

	return "", fmt.Errorf("PVC %s not found in namespace %s", pvcName, namespace)
}