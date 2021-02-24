package test

import (
	"context"
	"io/ioutil"

	crd "github.com/trilioData/k8s-triliovault/api/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func CreateObjectFromFile(ctx context.Context, cl client.Client, gvk *schema.GroupVersionKind, file, ns string) error {
	obj := GetObjectFromFile(gvk, file, ns)
	return cl.Create(ctx, &obj)
}
func DeleteObjectFromFile(ctx context.Context, cl client.Client, gvk *schema.GroupVersionKind, file, ns string) error {
	obj := GetObjectFromFile(gvk, file, ns)
	return cl.Delete(ctx, &obj)
}

func GetObjectFromFile(gvk *schema.GroupVersionKind, file, ns string) unstructured.Unstructured {
	obj := unstructured.Unstructured{}
	obj.SetGroupVersionKind(*gvk)
	bytes, _ := ioutil.ReadFile(file)
	_ = yaml.Unmarshal(bytes, &obj)
	if ns != "" {
		obj.SetNamespace(ns)
	}
	return obj
}

func SetBackupPlanStatus(backupPlanName, ns string, reqStatus crd.Status,
	cl client.Client) error {
	var appCR crd.BackupPlan
	if err := cl.Get(context.Background(), types.NamespacedName{
		Namespace: ns,
		Name:      backupPlanName,
	}, &appCR); err != nil {
		return err
	}
	appCR.Status.Status = reqStatus
	return cl.Status().Update(context.Background(), &appCR)
}
