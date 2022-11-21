/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	commonv1alpha1 "github.com/pfrybar/statefulgroup-operator/api/v1alpha1"
)

const ownerLabel = "pfrybarger.com/owner"

// StatefulGroupReconciler reconciles a StatefulGroup object
type StatefulGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=pfrybarger.com,resources=statefulgroups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=pfrybarger.com,resources=statefulgroups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=pfrybarger.com,resources=statefulgroups/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the StatefulGroup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *StatefulGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Reconciling...")

	var statefulGroup commonv1alpha1.StatefulGroup
	if err := r.Get(ctx, req.NamespacedName, &statefulGroup); err != nil {
		log.Error(err, "Unable to fetch StatefulGroup")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var serviceList corev1.ServiceList
	serviceListOptions := []client.ListOption{
		client.InNamespace(req.Namespace),
		client.MatchingLabels(map[string]string{ownerLabel: statefulGroup.Name}),
	}
	if err := r.List(ctx, &serviceList, serviceListOptions...); err != nil {
		log.Error(err, "Unable to list child Services")
		return ctrl.Result{}, err
	}

	var statefulSetList appsv1.StatefulSetList
	statefulSetListOptions := []client.ListOption{
		client.InNamespace(req.Namespace),
		client.MatchingLabels(map[string]string{ownerLabel: statefulGroup.Name}),
	}
	if err := r.List(ctx, &statefulSetList, statefulSetListOptions...); err != nil {
		log.Error(err, "Unable to list child StatefulSets")
		return ctrl.Result{}, err
	}

	// correlate the services + stateful sets
	items := map[string]StatefulGroupItem{}
	for _, service := range serviceList.Items {
		items[service.Name] = StatefulGroupItem{
			Name:    service.Name,
			Service: service.DeepCopy(),
		}
	}

	for _, statefulSet := range statefulSetList.Items {
		// small chance the stateful set doesn't exist (error condition)
		if item, ok := items[statefulSet.Name]; ok {
			item.StatefulSet = statefulSet.DeepCopy()
			items[statefulSet.Name] = item
		} else {
			items[statefulSet.Name] = StatefulGroupItem{
				Name:        statefulSet.Name,
				StatefulSet: statefulSet.DeepCopy(),
			}
		}
	}

	ready := []StatefulGroupItem{}
	notReady := []StatefulGroupItem{}

	toCreate := []StatefulGroupItem{}
	toFix := []StatefulGroupItem{}
	toUpdate := []StatefulGroupItem{}
	toDelete := []StatefulGroupItem{}

	numWanted := int(*statefulGroup.Spec.Replicas)
	for i := 0; i < numWanted; i++ {
		name := fmt.Sprintf("%s-%d", statefulGroup.Name, i)

		if item, ok := items[name]; ok {
			if groupIsReady(&item) {
				ready = append(ready, item)
			} else {
				notReady = append(notReady, item)
			}

			if item.Service == nil || item.StatefulSet == nil {
				toFix = append(toFix, item)
			} else {
				update := false

				serviceSpec := createServiceSpec(name, statefulGroup)
				statefulSetSpec := createStatefulSetSpec(name, statefulGroup)

				// work around since 'DeepDerivative' doesn't handle missing ints (which default to 0)
				for i, port := range serviceSpec.Ports {
					if port.TargetPort.IntVal == 0 && port.TargetPort.StrVal == "" {
						// target port not set, set the default value
						port.TargetPort = intstr.IntOrString{IntVal: port.Port}
						serviceSpec.Ports[i] = port
					}
				}

				// note: the ordering of arguments to 'DeepDerivative' are important
				if !equality.Semantic.DeepDerivative(serviceSpec, &item.Service.Spec) {
					item.Service.Spec = *serviceSpec
					update = true
				}

				// note: the ordering of arguments to 'DeepDerivative' are important
				if !equality.Semantic.DeepDerivative(statefulSetSpec, &item.StatefulSet.Spec) {
					item.StatefulSet.Spec = *statefulSetSpec
					update = true
				}

				if update {
					toUpdate = append(toUpdate, item)
				}
			}

			// remove the item so we can track the ones which need to be deleted
			delete(items, name)
		} else {
			item := StatefulGroupItem{
				Name:        name,
				Service:     createService(name, statefulGroup),
				StatefulSet: createStatefulSet(name, statefulGroup),
			}

			toCreate = append(toCreate, item)
		}
	}

	// anything remaining should be deleted
	for _, item := range items {
		toDelete = append(toDelete, item)
	}

	// TODO: deal with 'toFix' items

	// always create everything at once, no matter the state
	for _, item := range toCreate {
		log.Info("Creating item", "name", item.Name)

		if err := ctrl.SetControllerReference(&statefulGroup, item.Service, r.Scheme); err != nil {
			log.Error(err, "Unable to set ownership")
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, item.Service); err != nil {
			log.Error(err, "Unable to create Service", "name", item.Name)
			return ctrl.Result{}, err
		}

		if err := ctrl.SetControllerReference(&statefulGroup, item.StatefulSet, r.Scheme); err != nil {
			log.Error(err, "Unable to set ownership")
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, item.StatefulSet); err != nil {
			log.Error(err, "Unable to create StatefulSet", "name", item.Name)
			return ctrl.Result{}, err
		}
	}

	// only update (or continue updating) if everything is 'ready'
	// TODO: this can be changed later on to allow parallel updates (e.g. 2 at a time)
	if len(notReady) == 0 {
		for _, item := range toUpdate {
			log.Info("Updating item", "name", item.Name)

			if err := r.Update(ctx, item.Service); err != nil {
				log.Error(err, "Unable to update Service", "name", item.Name)
				return ctrl.Result{}, err
			}

			if err := r.Update(ctx, item.StatefulSet); err != nil {
				log.Error(err, "Unable to update StatefulSet", "name", item.Name)
				return ctrl.Result{}, err
			}

			break
		}
	}

	// only delete if nothing else is being done
	if len(notReady) == 0 && len(toCreate) == 0 && len(toUpdate) == 0 {
		for _, item := range toDelete {
			log.Info("Deleting item", "name", item.Name)

			if err := r.Delete(ctx, item.Service); err != nil {
				log.Error(err, "Unable to delete Service", "name", item.Name)
				return ctrl.Result{}, err
			}

			if err := r.Delete(ctx, item.StatefulSet); err != nil {
				log.Error(err, "Unable to delete StatefulSet", "name", item.Name)
				return ctrl.Result{}, err
			}
		}
	}

	replicas := int32(len(ready))
	statefulGroup.Status.Replicas = &replicas

	if err := r.Status().Update(ctx, &statefulGroup); err != nil {
		log.Error(err, "Unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

type StatefulGroupItem struct {
	Name        string
	Service     *corev1.Service
	StatefulSet *appsv1.StatefulSet
}

func groupIsReady(item *StatefulGroupItem) bool {
	// it's difficult to determine if a service is ready, assume it's ready if it's been created
	// if the observed generation of a stateful set is different, then it hasn't reacted to spec changes yet
	return item.Service != nil &&
		item.StatefulSet != nil &&
		item.StatefulSet.Generation == item.StatefulSet.Status.ObservedGeneration &&
		*item.StatefulSet.Spec.Replicas == item.StatefulSet.Status.ReadyReplicas &&
		*item.StatefulSet.Spec.Replicas == item.StatefulSet.Status.UpdatedReplicas
}

func createServiceSpec(name string, statefulGroup commonv1alpha1.StatefulGroup) *corev1.ServiceSpec {
	selectorKey := statefulGroup.Spec.SelectorLabelKey
	spec := statefulGroup.Spec.ServiceTemplate.DeepCopy()
	spec.Selector = map[string]string{selectorKey: name}
	return spec
}

func createService(name string, statefulGroup commonv1alpha1.StatefulGroup) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{ownerLabel: statefulGroup.Name},
			Annotations: map[string]string{},
			Name:        name,
			Namespace:   statefulGroup.Namespace,
		},
		Spec: *createServiceSpec(name, statefulGroup),
	}
}

func createStatefulSetSpec(name string, statefulGroup commonv1alpha1.StatefulGroup) *appsv1.StatefulSetSpec {
	selectorKey := statefulGroup.Spec.SelectorLabelKey
	spec := statefulGroup.Spec.StatefulSetTemplate.DeepCopy()
	spec.ServiceName = name

	spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{selectorKey: name},
	}

	if spec.Template.Labels == nil {
		spec.Template.Labels = map[string]string{}
	}

	spec.Template.Labels[selectorKey] = name

	return spec
}

func createStatefulSet(name string, statefulGroup commonv1alpha1.StatefulGroup) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{ownerLabel: statefulGroup.Name},
			Annotations: map[string]string{},
			Name:        name,
			Namespace:   statefulGroup.Namespace,
		},
		Spec: *createStatefulSetSpec(name, statefulGroup),
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *StatefulGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&commonv1alpha1.StatefulGroup{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Complete(r)
}