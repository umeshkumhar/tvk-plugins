package k8sobjects

import (
	corev1 "k8s.io/api/core/v1"
)

type PV struct {
	Name          string
	Annotations   map[string]string
	ReclaimPolicy corev1.PersistentVolumeReclaimPolicy
	PvcSpec       *PVC
	Namespace     string
}

// func (pv *PV) CreateTemplate() *corev1.PersistentVolume {

// 	// PV spec, similar to a PV manifest
// 	pvSpec := &corev1.PersistentVolume{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:        pv.Name,
// 			Annotations: pv.Annotations,
// 		},
// 		Spec: corev1.PersistentVolumeSpec{
// 			PersistentVolumeReclaimPolicy: pv.ReclaimPolicy,
// 			AccessModes:                   options.PvcSpec.Spec.AccessModes,
// 			Capacity: corev1.ResourceList{
// 				v1.ResourceName(v1.ResourceStorage): options.PvcSpec.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)],
// 			},
// 			PersistentVolumeSource: v1.PersistentVolumeSource{
// 				FlexVolume: &v1.FlexPersistentVolumeSource{
// 					Driver: driver,
// 					FSType: fsType,
// 					// Provide the name of the secret
// 					// if you are using one
// 					SecretRef: &v1.SecretReference{
// 						Name:      os.Getenv("secret-name"),
// 						Namespace: options.PVC.Namespace,
// 					},
// 					ReadOnly: false,
// 					Options:  map[string]string{"volumeId": volumeId},
// 				},
// 			},
// 		},
// 	}

// 	log.Debugf("created a PVC with definition: %s", pvcSpec)
// 	return pvcSpec
// }
