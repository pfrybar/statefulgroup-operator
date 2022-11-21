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

package v1alpha1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// StatefulGroupSpec defines the desired state of StatefulGroup
type StatefulGroupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Replicas            *int32                 `json:"replicas,omitempty"`
	SelectorLabelKey    string                 `json:"selectorLabelKey,omitempty"`
	ServiceTemplate     corev1.ServiceSpec     `json:"serviceTemplate,omitempty"`
	StatefulSetTemplate appsv1.StatefulSetSpec `json:"statefulSetTemplate,omitempty"`
}

// StatefulGroupStatus defines the observed state of StatefulGroup
type StatefulGroupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Replicas *int32 `json:"replicas,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=stg
//+kubebuilder:subresource:status
//+kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
//+kubebuilder:printcolumn:name="Wanted",type=integer,JSONPath=`.spec.replicas`
//+kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.replicas`

// StatefulGroup is the Schema for the statefulgroups API
type StatefulGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StatefulGroupSpec   `json:"spec,omitempty"`
	Status StatefulGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StatefulGroupList contains a list of StatefulGroup
type StatefulGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StatefulGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StatefulGroup{}, &StatefulGroupList{})
}
