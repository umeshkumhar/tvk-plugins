package restore

import (
	"context"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1 "github.com/trilioData/k8s-triliovault/api/v1"
	controllerHelpers "github.com/trilioData/k8s-triliovault/controllers/helpers"
	"github.com/trilioData/k8s-triliovault/internal"
)

// Reconciler reconciles a Restore object
type Reconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=triliovault.trilio.io,resources=restores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=triliovault.trilio.io,resources=restores/status,verbs=get;update;patch

// nolint:gocyclo // for future ref
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("restore", req.NamespacedName)

	if !controllerHelpers.ContinueReconcile(req.Namespace) {
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling restore")

	// Load the Restore by namespace name
	var restore v1.Restore
	if err := r.Get(ctx, req.NamespacedName, &restore); err != nil {
		r.Recorder.Eventf(&restore, corev1.EventTypeWarning, "RestoreRetrievalFailed",
			"Retrieval of Restore: %s in namespace %s, Failed", restore.Name, restore.Namespace)
		log.Info("Unable to fetch restore", "error", err.Error())
		return ctrl.Result{}, controllerHelpers.IgnoreNotFound(err)
	}

	continueReconcile, err := reconcileRestoreDeleteFinalizer(ctx, r, &restore)
	if !continueReconcile {
		return ctrl.Result{}, err
	}

	// Return Completed restore
	if restore.Status.Status == v1.Completed || restore.Status.Status == v1.Failed {
		log.Info("Skipping reconcile for final state of restore", "status", restore.Status.Status)
		return ctrl.Result{}, nil
	}

	// Set initial status of a restore
	if restore.Status.Status == "" || restore.Status.Status == v1.Pending {
		log.Info("Setting initial status of a restore", "status", restore.Status.Status)

		rErr := controllerHelpers.SetupRBACAuthorization(ctx, r.Client, restore.UID, restore.Spec.RestoreNamespace,
			internal.RestoreKind)
		if rErr != nil {
			log.Error(rErr, "Error while setting up rbac authorization for restore operation")
			return ctrl.Result{}, rErr
		}

		restore.Status.StartTimestamp = &metav1.Time{Time: time.Now()}
		restore.Status.Status = v1.InProgress
	}

	childJobs, err := getRestoreChildJobs(ctx, r, &restore)
	if err != nil {
		return ctrl.Result{}, err
	}
	metadataValidationJob, dataRestoreJobMap, metadataRestoreJob, hookUnQuiesceJob := SegregateRestoreChildJobs(childJobs)
	log.Info("Segregated restore child jobs", "validation", metadataValidationJob != nil,
		"data", len(dataRestoreJobMap), "metadata", metadataRestoreJob != nil)

	if restore.Status.Status == v1.Failed {
		tearDownRestoreRBACAuth(ctx, r, &restore, childJobs)
		log.Info("Skipping reconcile for failed state of restore", "status", restore.Status.Status)
		return ctrl.Result{}, nil
	}

	// Check Validation Job
	log.Info("Checking restore validation job")
	metadataValidationStatus := checkRestoreJob(ctx, r, &restore, v1.RestoreValidation, metadataValidationJob)
	if !metadataValidationStatus.Completed {
		err = updateRestoreStatus(ctx, r, &restore, false)
		return ctrl.Result{}, err
	}

	// Check Data Restore Job
	log.Info("Checking restore data jobs")
	dataRestoreStatus, dErr := checkRestoreDataJob(ctx, r, &restore, dataRestoreJobMap)
	if dErr != nil {
		return ctrl.Result{}, dErr
	}
	restore.Status.PercentageCompletion = getRestorePercentage(dataRestoreStatus.TotalCount,
		dataRestoreStatus.CompletedCount, false, false)
	restore.Status.Size = dataRestoreStatus.Size

	if !dataRestoreStatus.Completed {
		err = updateRestoreStatus(ctx, r, &restore, dataRestoreStatus.Active)
		return ctrl.Result{}, err
	}

	// Check Metadata Restore Job
	log.Info("Checking restore metadata job")
	metadataRestoreStatus := checkRestoreJob(ctx, r, &restore, v1.MetadataRestore, metadataRestoreJob)
	restore.Status.PercentageCompletion = getRestorePercentage(dataRestoreStatus.TotalCount,
		dataRestoreStatus.CompletedCount, metadataRestoreStatus.Completed, false)
	restore.Status.Size = dataRestoreStatus.Size
	if !metadataRestoreStatus.Completed {
		err = updateRestoreStatus(ctx, r, &restore, false)
		return ctrl.Result{}, err
	}

	if restore.Spec.HookConfig != nil {
		// Check Restore Hook UnQuiesce Job
		log.Info("Checking restore metadata job")
		hookUnQuiesceStatus := checkRestoreJob(ctx, r, &restore, v1.UnquiesceRestore, hookUnQuiesceJob)
		restore.Status.PercentageCompletion = getRestorePercentage(dataRestoreStatus.TotalCount,
			dataRestoreStatus.CompletedCount, metadataRestoreStatus.Completed, hookUnQuiesceStatus.Completed)
		restore.Status.Size = dataRestoreStatus.Size
		if !hookUnQuiesceStatus.Completed {
			err = updateRestoreStatus(ctx, r, &restore, false)
			return ctrl.Result{}, err
		}
	}

	// Setting final Completed status of a restore
	restore.Status.Status = v1.Completed
	// Setting final Percentage of completion
	restore.Status.PercentageCompletion = 100
	err = updateRestoreStatus(ctx, r, &restore, false)
	if err != nil {
		return ctrl.Result{}, err
	}

	if restore.Status.Status == v1.Completed {
		controllerHelpers.CleanupJobs(ctx, r.Log, r.Client, childJobs.Items, false)
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, err
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	log := r.Log.WithValues("function", "SetupWithManager")

	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &batchv1.Job{}, internal.ControllerFieldSelector,
		func(rawObj client.Object) []string {
			job := rawObj.(*batchv1.Job)
			return []string{job.GetLabels()[internal.ControllerOwnerUID]}
		}); err != nil {
		return err
	}

	createFunction := func(e event.CreateEvent) bool {
		if e.Object == nil {
			log.Error(nil, "Create event has no metadata", "event", e)
			return false
		}
		if e.Object == nil {
			log.Error(nil, "Create event has no runtime object to create", "event", e)
			return false
		}
		return true
	}

	deleteFunction := func(e event.DeleteEvent) bool {
		if e.Object == nil {
			log.Error(nil, "Delete event has no metadata", "event", e)
			return false
		}
		if e.Object == nil {
			log.Error(nil, "Delete event has no runtime object to delete", "event", e)
			return false
		}
		// Accept delete events only for batchv1.Job object
		return true
	}

	updateFunction := func(e event.UpdateEvent) bool {
		if !controllerHelpers.CheckUpdateEventPredicate(e, r.Log) {
			return false
		}
		if currentRestoreObject, ok := e.ObjectNew.DeepCopyObject().(*v1.Restore); ok {
			previousRestoreObject := e.ObjectOld.DeepCopyObject().(*v1.Restore)
			if !reflect.DeepEqual(currentRestoreObject.Status, previousRestoreObject.Status) {
				log.Info("Skipping update event for restore status update", "name", e.ObjectNew.GetName())
				return false
			}
		}
		// Accept the events for update on Job statuses
		return true
	}

	p := predicate.Funcs{
		CreateFunc: createFunction,
		DeleteFunc: deleteFunction,
		UpdateFunc: updateFunction,
	}

	handlerFunc := func(a client.Object) []reconcile.Request {
		annotations := a.GetAnnotations()
		controllerName, controllerNamePresent := annotations[internal.ControllerOwnerName]
		controllerNamespace, controllerNamespacePresent := annotations[internal.ControllerOwnerNamespace]
		if controllerNamePresent && controllerNamespacePresent {
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: controllerNamespace,
				Name: controllerName}}}
		}
		return nil
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Restore{}).
		WithEventFilter(p).
		Watches(&source.Kind{Type: &batchv1.Job{}}, handler.EnqueueRequestsFromMapFunc(handlerFunc)).
		Complete(r)
}
