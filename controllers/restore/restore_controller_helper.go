package restore

import (
	"context"
	"fmt"
	"math"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/trilioData/k8s-triliovault/api/v1"
	controllerHelpers "github.com/trilioData/k8s-triliovault/controllers/helpers"
	"github.com/trilioData/k8s-triliovault/internal"
	"github.com/trilioData/k8s-triliovault/internal/helpers"
	"github.com/trilioData/k8s-triliovault/pkg/metamover/decorator"
)

const (
	RestoreDataWeightage     = 0.6
	RestoreMetadataWeightage = 0.3
	RestoreHookWeightage     = 0.1
)

func getRestoreChildJobs(ctx context.Context, r *Reconciler, restore *v1.Restore) (*batchv1.JobList, error) {
	log := r.Log.WithValues("restore", types.NamespacedName{Namespace: restore.Namespace, Name: restore.Name},
		"function", "getRestoreChildJobs")
	childJobs := &batchv1.JobList{}
	log.Info("Fetching restore child jobs")
	if err := r.List(ctx, childJobs, client.InNamespace(restore.Spec.RestoreNamespace),
		client.MatchingFields{internal.ControllerFieldSelector: string(restore.GetUID())}); err != nil {
		log.Error(err, "Error while listing child Jobs")
		return childJobs, err
	}
	log.Info("Found restore child jobs", "count", len(childJobs.Items))
	return childJobs, nil
}

// Returns Child jobs of a restore job based on phases
func SegregateRestoreChildJobs(childJobs *batchv1.JobList) (metadataValidationJob *batchv1.Job,
	dataRestoreJobMap map[string]*batchv1.Job, metadataRestoreJob, hookUnQuiesceJob *batchv1.Job) {
	dataRestoreJobMap = make(map[string]*batchv1.Job)

	for i := 0; i < len(childJobs.Items); i++ {
		childJob := childJobs.Items[i]
		jobOperation := childJob.Annotations[internal.Operation]
		switch jobOperation {
		case internal.MetadataRestoreValidationOperation:
			metadataValidationJob = &childJob
		case internal.DataRestoreOperation:
			hash := helpers.GetHash(childJob.Annotations[internal.AppComponent], childJob.Annotations[internal.ComponentIdentifier],
				childJob.Annotations[internal.RestorePVCName])
			dataRestoreJobMap[hash] = &childJob
		case internal.MetadataRestoreOperation:
			metadataRestoreJob = &childJob
		case internal.UnquiesceOperation:
			hookUnQuiesceJob = &childJob

		}

	}

	return metadataValidationJob, dataRestoreJobMap, metadataRestoreJob, hookUnQuiesceJob
}

func GetRestoreJobLabels(restore *v1.Restore) map[string]string {
	labels := map[string]string{internal.ControllerOwnerUID: string(restore.GetUID())}

	return labels
}

func GetRestoreJobAnnotations(restore *v1.Restore, operation string) map[string]string {
	annotations := map[string]string{
		internal.ControllerOwnerName:      restore.Name,
		internal.ControllerOwnerNamespace: restore.Namespace,
		internal.Operation:                operation,
	}

	return annotations
}

// Create child jobs for restore data with DataMover Pod
func createDataRestoreJobs(ctx context.Context, r *Reconciler, restore *v1.Restore,
	nonChildJobRestoreDataComponents []helpers.ApplicationDataSnapshot) error {

	log := r.Log.WithValues("function", "createDataRestoreJobs")

	restoreNamespace := restore.Spec.RestoreNamespace

	for i := 0; i < len(nonChildJobRestoreDataComponents); i++ {
		appDs := nonChildJobRestoreDataComponents[i]
		restoreDataSnapshot := appDs.DataComponent

		// Get PVC of a data component
		var pvc corev1.PersistentVolumeClaim
		pvc, err := internal.GetPVCStruct(restoreDataSnapshot.PersistentVolumeClaimMetadata)
		if err != nil {
			log.Error(err, "Error while getting pvc")
			return err
		}
		unstructPVC, err := CleanupPVC(&pvc)
		if err != nil {
			log.Error(err, "Error while cleaning up PVC for Data Mover restore")
			return err
		}
		unstructPVC.SetNamespace(restoreNamespace)
		unstructPVC.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind(internal.PersistentVolumeClaimKind))
		err = r.Client.Create(ctx, &unstructPVC)
		if err != nil {
			log.Error(err, "Error while creating the PVC for Data Mover restore")
			return err
		}

		annotations := GetRestoreJobAnnotations(restore, internal.DataRestoreOperation)
		annotations[internal.AppComponent] = string(appDs.AppComponent)
		annotations[internal.ComponentIdentifier] = appDs.ComponentIdentifier
		annotations[internal.RestorePVCName] = pvc.Name
		dataRestoreContainer := controllerHelpers.GetRestoreDatamoverContainer(restore.Namespace, restore.Name,
			restore.Spec.Source.Target.Name, &pvc, &appDs)
		serviceAccountName := controllerHelpers.GetAuthResourceName(restore.UID, internal.RestoreKind)
		dataRestoreJob := controllerHelpers.GetJob(restore.Name, restoreNamespace, dataRestoreContainer,
			controllerHelpers.GetDatamoverVolumes(&pvc, false), serviceAccountName)
		// Add init container to find block device path
		if pvc.Spec.VolumeMode != nil && *pvc.Spec.VolumeMode == corev1.PersistentVolumeBlock {
			dataRestoreJob.Spec.Template.Spec.InitContainers = []corev1.Container{*controllerHelpers.GetBlockDeviceInitContainer()}
		}
		// Warning: Do not overwrite the labels which are getting set in GetJob function
		ownerLabels := GetRestoreJobLabels(restore)
		for k, v := range ownerLabels {
			dataRestoreJob.Labels[k] = v
		}
		dataRestoreJob.SetAnnotations(annotations)
		err = r.Client.Create(ctx, dataRestoreJob)
		if err != nil {
			log.Error(err, "Error while creating the DataRestoreJob")
			r.Recorder.Eventf(restore, corev1.EventTypeWarning, "DataRestoreJobsCreationFailed",
				"Data restore job creation Failed for PVC: %s and created for %s", pvc.Name, i)
			return err
		}
	}

	if len(nonChildJobRestoreDataComponents) > 0 {
		r.Recorder.Eventf(restore, corev1.EventTypeNormal, "DataRestoreJobsCreated",
			"Data restore jobs created: %s", len(nonChildJobRestoreDataComponents))
	}

	return nil
}

// createRestoreJob creates restore job like validation, metadataRestore, Hook UnQuiesce
func createRestoreJob(ctx context.Context, r *Reconciler, restore *v1.Restore, phase v1.RestorePhase) error {
	var (
		annotations map[string]string
		container   *corev1.Container
	)
	log := r.Log.WithValues("function", "createRestoreJob")

	serviceAccountName := controllerHelpers.GetAuthResourceName(restore.UID, internal.RestoreKind)

	switch phase {
	case v1.RestoreValidation:
		annotations = GetRestoreJobAnnotations(restore, internal.MetadataRestoreValidationOperation)
		container = controllerHelpers.GetMetaValidationContainer(restore.Namespace, restore.Name, restore.Spec.Source.Target.Name)
	case v1.MetadataRestore:
		annotations = GetRestoreJobAnnotations(restore, internal.MetadataRestoreOperation)
		container = controllerHelpers.GetMetaRestoreContainer(restore.Namespace, restore.Name, restore.Spec.Source.Target.Name)
	case v1.UnquiesceRestore:
		annotations = GetRestoreJobAnnotations(restore, internal.UnquiesceOperation)
		container = controllerHelpers.GetHookContainer(restore.Namespace, internal.UnquiesceOperation, restore.Name, internal.RestoreKind)
	}

	restoreJob := controllerHelpers.GetJob(restore.Name, restore.Spec.RestoreNamespace, container, []corev1.Volume{}, serviceAccountName)
	// Warning: Do not overwrite the labels which are getting set in GetJob function
	ownerLabels := GetRestoreJobLabels(restore)
	for k, v := range ownerLabels {
		restoreJob.Labels[k] = v
	}
	restoreJob.SetAnnotations(annotations)
	err := r.Client.Create(ctx, restoreJob)
	if err != nil {
		log.Error(err, fmt.Sprintf("error while creating %s job", string(phase)))
		return err
	}
	r.Recorder.Eventf(restore, corev1.EventTypeNormal, fmt.Sprintf("%sJobCreated", string(phase)),
		"%s job created: %s", phase, restoreJob.Name)

	return nil
}

// Returns percentage of restore Completed based on restore phases and data restore completion
func getRestorePercentage(restoreDataComponentsCount, completedRestoreDataComponentsCount int, metadataRestoreCompleted,
	hookUnQuiesceCompleted bool) int8 {
	progress := 0.0

	// If restore has data components
	if restoreDataComponentsCount != 0 {
		dataComponentProgress := float64(completedRestoreDataComponentsCount) / float64(restoreDataComponentsCount)
		progress += RestoreDataWeightage * dataComponentProgress

		if metadataRestoreCompleted {
			progress += RestoreMetadataWeightage
		}

		if hookUnQuiesceCompleted {
			progress += RestoreHookWeightage
		}
	} else if metadataRestoreCompleted {
		progress = 0.7
		if hookUnQuiesceCompleted {
			progress += 3 * RestoreHookWeightage
		}
	}

	return int8(math.Ceil(progress * 100))
}

func reconcileRestoreDeleteFinalizer(ctx context.Context, r *Reconciler,
	restore *v1.Restore) (continueReconcile bool, err error) {

	log := r.Log.WithValues("function", "reconcileRestoreDeleteFinalizer")

	if restore.ObjectMeta.DeletionTimestamp.IsZero() {
		if !internal.ContainsString(restore.ObjectMeta.Finalizers, internal.ChildDeleteFinalizer) {
			restore.ObjectMeta.Finalizers = append(restore.ObjectMeta.Finalizers, internal.ChildDeleteFinalizer)
			if err := r.Update(context.Background(), restore); err != nil {
				log.Error(err, "Error while updating finalizer")
				return continueReconcile, err
			}
		}
	} else {
		if internal.ContainsString(restore.ObjectMeta.Finalizers, internal.ChildDeleteFinalizer) {
			childJobs, err := getRestoreChildJobs(ctx, r, restore)
			if err != nil {
				log.Error(err, "Error while listing restore child jobs")
				return continueReconcile, err
			}
			dErr := deleteJobs(ctx, r.Client, childJobs)
			if dErr != nil {
				log.Error(dErr, "Error while deleting restore child jobs")
				return continueReconcile, dErr
			}
			restore.ObjectMeta.Finalizers = internal.RemoveString(restore.ObjectMeta.Finalizers, internal.ChildDeleteFinalizer)
			if err := r.Update(context.Background(), restore); err != nil {
				log.Error(err, "Error while updating finalizer")
				return continueReconcile, err
			}
			return continueReconcile, nil
		}
	}
	return true, nil
}

func deleteJobs(ctx context.Context, cli client.Client, jobList *batchv1.JobList) error {
	propagationPolicy := metav1.DeletePropagationForeground
	for index := range jobList.Items {
		job := jobList.Items[index]
		err := cli.Delete(ctx, &job, &client.DeleteOptions{PropagationPolicy: &propagationPolicy})
		if err != nil {
			return err
		}
	}
	return nil
}

func checkRestoreJob(ctx context.Context, r *Reconciler, restore *v1.Restore,
	phase v1.RestorePhase, job *batchv1.Job) controllerHelpers.JobStatus {

	restore.Status.Phase = phase
	restore.Status.PhaseStatus = v1.InProgress

	status := controllerHelpers.JobStatus{}
	log := r.Log.WithValues("function", "checkRestoreJob")
	// Check status of Restore job depending upon phase Job

	if job != nil {
		status = controllerHelpers.GetJobStatus(job)
		log.Info("Job status", "Active", status.Active,
			"Completed", status.Completed, "Failed", status.Failed)
		restore.Status.PhaseStatus = controllerHelpers.GetJobPhaseStatus(status)
		implementRestoreConditions(restore, phase, v1.InProgress, "")
		if status.Failed {
			restore.Status.Status = v1.Failed
			implementRestoreConditions(restore, phase, v1.Failed, "")
		}
		if status.Completed {
			implementRestoreConditions(restore, phase, v1.Completed, "")
		}
		return status
	}

	err := createRestoreJob(ctx, r, restore, phase)
	if err != nil {
		log.Error(err, fmt.Sprintf("Error while creating %s job", phase))
		restore.Status.PhaseStatus = v1.Failed
		restore.Status.Status = v1.Failed
		implementRestoreConditions(restore, phase, v1.Failed, "Error while creating job")
		return status
	}
	implementRestoreConditions(restore, phase, v1.InProgress, "")
	log.Info(fmt.Sprintf("Created %s job", phase))

	return status
}

func checkRestoreDataJob(ctx context.Context, r *Reconciler, restore *v1.Restore,
	dataRestoreJobMap map[string]*batchv1.Job) (status *controllerHelpers.DataComponentListStatus, err error) {
	// Aggregate Data Snapshots found in restore application
	log := r.Log.WithValues("function", "checkRestoreDataJob")

	restoreDataComponents := controllerHelpers.GetRestoreApplicationDataComponents(restore.Status.RestoreApplication)
	log.Info("Found Restore Application DataComponents", "count", len(restoreDataComponents))

	if len(restoreDataComponents) == 0 {
		return &controllerHelpers.DataComponentListStatus{
			Completed: true,
		}, err
	}

	restoreDataComponents = controllerHelpers.DeDuplicateDataSnapshot(restoreDataComponents)
	log.Info("DeDuplicate Restore Application DataComponents", "count", len(restoreDataComponents))

	status = controllerHelpers.GetDataComponentListStatus(restoreDataComponents, dataRestoreJobMap)
	dataRestoreActive, dataRestoreCompleted, dataRestoreFailed := status.Active,
		status.Completed, status.Failed
	nonChildJobRestoreDataComponents := status.NonChildJobDataComponents
	log.Info("Restore Application DataComponents status", "Active", dataRestoreActive,
		"Completed", dataRestoreCompleted, "Failed", dataRestoreFailed, "NonChildJobDataComponents",
		len(nonChildJobRestoreDataComponents), "Completed-count", status.CompletedCount,
		"restore-size", status.Size)

	implementDataRestoreConditions(restore, restoreDataComponents)

	if len(restoreDataComponents) != 0 {
		restore.Status.Phase = v1.DataRestore
		jobStatus := controllerHelpers.JobStatus{Active: dataRestoreActive, Completed: dataRestoreCompleted, Failed: dataRestoreFailed}
		restore.Status.PhaseStatus = controllerHelpers.GetJobPhaseStatus(jobStatus)
		restore.Status.Size = status.Size
	}

	if dataRestoreFailed {
		restore.Status.Status = v1.Failed
		implementRestoreConditions(restore, v1.DataRestore, v1.Failed, "")
		return status, err
	}

	if dataRestoreCompleted {
		implementRestoreConditions(restore, v1.DataRestore, v1.Completed, "")
		return status, err
	}

	err = createDataRestoreJobs(ctx, r, restore, nonChildJobRestoreDataComponents)
	if err != nil {
		log.Error(err, "Error while creating data restore jobs")
		restore.Status.PhaseStatus = v1.Error
		restore.Status.Status = v1.Error
		if !apierrs.IsBadRequest(err) {
			restore.Status.PhaseStatus = v1.Failed
			restore.Status.Status = v1.Failed
			implementRestoreConditions(restore, v1.DataRestore, v1.Failed, "")
			return status, nil
		}
		return status, err
	}
	implementRestoreConditions(restore, v1.DataRestore, v1.InProgress, "")

	return status, err
}

func updateRestoreStatus(ctx context.Context, r *Reconciler, restore *v1.Restore, skipAuthTearDown bool) error {

	log := r.Log.WithValues("restore", types.NamespacedName{Namespace: restore.Namespace, Name: restore.Name},
		"function", "createRestoreChildJobs")

	if restore.Status.Status == v1.Completed || restore.Status.Status == v1.Failed {
		restore.Status.CompletionTimestamp = &metav1.Time{Time: time.Now()}
	}

	log.Info("Updating restore status")
	// Update restore status in the client object

	sErr := r.Status().Update(ctx, restore)
	if sErr != nil {
		r.Recorder.Eventf(restore, corev1.EventTypeWarning, "RestoreUpdateFailed",
			"Updating Restore: %s, Failed", restore.Name, restore.Namespace)
		log.Error(sErr, "Unable to update restore")
		return controllerHelpers.IgnoreNotFound(sErr)
	}

	if restore.Status.Status == v1.Completed || (restore.Status.Status == v1.Failed && !skipAuthTearDown) {
		rErr := controllerHelpers.TearDownRBACAuthorization(ctx, r.Client, restore.UID, restore.Spec.RestoreNamespace, internal.RestoreKind)
		if rErr != nil {
			log.Error(rErr, "Error while tearing up rbac authorization for restore operation")
			utilruntime.HandleError(rErr)
		}
	}

	return nil
}

func tearDownRestoreRBACAuth(ctx context.Context, r *Reconciler, restore *v1.Restore,
	restoreChildJobs *batchv1.JobList) {

	log := r.Log.WithValues("function", " tearDownRestoreRBACAuth")

	restoreJobListStatus := controllerHelpers.GetJobsListStatus(restoreChildJobs)
	log.Info("Checking status of Restore Jobs List", "active", restoreJobListStatus.Active,
		"failed", restoreJobListStatus.Failed, "completed", restoreJobListStatus.Completed)
	if controllerHelpers.IsJobListCompleted(restoreChildJobs, restoreJobListStatus) {
		log.Info("Tearing Down RBAC Authorization of finalized restore")
		err := controllerHelpers.TearDownRBACAuthorization(ctx, r.Client, restore.UID,
			restore.Spec.RestoreNamespace, internal.RestoreKind)
		if err != nil {
			log.Error(err, "Error tear down RBAC authorization")
			utilruntime.HandleError(err)
		}
	}
}

// TODO: Update PVC Storage Class with the one present in the cluster
// CleanupPVC remove cluster state and status of kubernetes pvc object
func CleanupPVC(pvc *corev1.PersistentVolumeClaim) (unstructured.Unstructured, error) {

	pvc.Spec.DataSource = nil
	pvc.Spec.VolumeName = ""
	pvc.ObjectMeta.Finalizers = []string{}
	pvc.SetAnnotations(map[string]string{})

	res := decorator.UnstructResource{}
	if err := res.ToUnstructured(pvc); err != nil {
		return unstructured.Unstructured{}, err
	}

	res.Cleanup()

	return unstructured.Unstructured(res), nil
}

// implementRestoreConditions will add current phase conditions for restore
// nolint:dupl // added to get rid of lint errors of duplicate code
func implementRestoreConditions(restore *v1.Restore, phase v1.RestorePhase, status v1.Status, specificReason string) {

	var (
		condition v1.RestoreCondition
	)

	condition.Phase = phase
	if specificReason != "" {
		condition.Reason = specificReason
	} else {
		condition.Reason = internal.CombinedReason(string(phase), string(status))
	}
	condition.Status = status
	condition.Timestamp = internal.CurrentTime()

	if len(restore.Status.Condition) > 0 {
		for i := len(restore.Status.Condition) - 1; i >= 0; i-- {
			c := restore.Status.Condition[i]
			if c.Phase == phase && c.Status == status {
				return
			}
		}
	}

	restore.Status.Condition = append(restore.Status.Condition, condition)
}

// implementDataRestoreConditions will add current data restore conditions for each data component
func implementDataRestoreConditions(restore *v1.Restore, restoreDataComponents []helpers.ApplicationDataSnapshot) {
	for i := range restoreDataComponents {
		resdatacom := restoreDataComponents[i]
		switch resdatacom.AppComponent {
		case internal.Custom:
			controllerHelpers.ImplementDataComponentConditions(restore.Status.RestoreApplication.Custom.Snapshot,
				resdatacom.Status, resdatacom.DataComponent.PersistentVolumeClaimName, v1.DataRestoreOperation)
		case internal.Helm:
			for helmIndex := range restore.Status.RestoreApplication.HelmCharts {
				helmChart := &restore.Status.RestoreApplication.HelmCharts[helmIndex]
				if helmChart.Snapshot.Release == resdatacom.ComponentIdentifier {
					controllerHelpers.ImplementDataComponentConditions(helmChart.Snapshot, resdatacom.Status,
						resdatacom.DataComponent.PersistentVolumeClaimName, v1.DataRestoreOperation)
				}
			}
		case internal.Operator:
			for opIndex := range restore.Status.RestoreApplication.Operators {
				operator := restore.Status.RestoreApplication.Operators[opIndex]
				if operator.Snapshot.OperatorID == resdatacom.ComponentIdentifier {
					controllerHelpers.ImplementDataComponentConditions(operator.Snapshot, resdatacom.Status,
						resdatacom.DataComponent.PersistentVolumeClaimName, v1.DataRestoreOperation)
				}
				if operator.Snapshot.Helm != nil {
					operatorHelmID := helpers.GetOperatorHelmIdentifier(operator.Snapshot.OperatorID, operator.Snapshot.Helm.Release)
					if operatorHelmID == resdatacom.ComponentIdentifier {
						controllerHelpers.ImplementDataComponentConditions(operator.Snapshot, resdatacom.Status,
							resdatacom.DataComponent.PersistentVolumeClaimName, v1.DataRestoreOperation)
					}
				}
			}
		}
	}
}
