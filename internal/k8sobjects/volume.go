package k8sobjects

import (
	corev1 "k8s.io/api/core/v1"
)

type Volume struct {
	Size string
	Name string
	Mode *corev1.PersistentVolumeMode
}
