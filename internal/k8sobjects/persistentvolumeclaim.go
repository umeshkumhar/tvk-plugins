package k8sobjects

import (
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PVC struct {
	Name             string
	StorageClassName *string
	VolumeSpec       *Volume
	AccessMode       corev1.PersistentVolumeAccessMode
	Namespace        string
}

func (pvc *PVC) CreateTemplate() *corev1.PersistentVolumeClaim {
	pvcSpec := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{pvc.AccessMode},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(pvc.VolumeSpec.Size),
				},
			},
			StorageClassName: pvc.StorageClassName,
			VolumeName:       pvc.VolumeSpec.Name,
			VolumeMode:       pvc.VolumeSpec.Mode,
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase:       corev1.ClaimBound,
			AccessModes: []corev1.PersistentVolumeAccessMode{pvc.AccessMode},
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(pvc.VolumeSpec.Size),
			},
		},
	}

	log.Debugf("created a PVC with definition: %s", pvcSpec)
	return pvcSpec
}
