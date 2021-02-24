package k8sobjects

import (
	log "github.com/sirupsen/logrus"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Deployment struct {
	Annotations map[string]string
	Name        string
	Namespace   string
	PodSpec     *Pod
}

func (d *Deployment) CreateTemplate() *appv1.Deployment {
	deployment := &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.Name,
			Namespace: d.Namespace,
			Labels:    d.PodSpec.PodObjectMeta.Labels,
		},
		Spec: appv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: d.PodSpec.PodObjectMeta.Labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      d.PodSpec.PodObjectMeta.Labels,
					Annotations: d.Annotations,
				},
				Spec: corev1.PodSpec{
					Containers: d.PodSpec.Spec.Containers,
				},
			},
		},
	}

	log.Debugf("created a Deployment with definition: %s", deployment)
	return deployment
}
