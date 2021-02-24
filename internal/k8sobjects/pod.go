package k8sobjects

import (
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodObjectMeta struct {
	Labels    map[string]string
	Name      string
	Namespace string
}

type PodSpec struct {
	Containers    []corev1.Container
	Volumes       []corev1.Volume
	RestartPolicy corev1.RestartPolicy
}

type Pod struct {
	PodObjectMeta
	Spec PodSpec
}

func (p *Pod) CreateTemplate() *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.PodObjectMeta.Name,
			Namespace: p.PodObjectMeta.Namespace,
			Labels:    p.PodObjectMeta.Labels,
		},

		Spec: corev1.PodSpec{
			Containers:    p.Spec.Containers,
			Volumes:       p.Spec.Volumes,
			RestartPolicy: p.Spec.RestartPolicy,
		},
	}
	log.Debugf("created a pod with definition: %s", pod)
	return pod
}
