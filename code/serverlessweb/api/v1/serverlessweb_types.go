/*
Copyright 2021.

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

package v1

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServerlessWebSpec defines the desired state of ServerlessWeb
type ServerlessWebSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	//业务服务对应的镜像，包括名称：tag版本
	Image string `json:"image"`
	//service占用的宿主机端口，外部请求通过此端口访问pod的服务
	Port *int32 `json:"port"`

	// 单个pod的QPS上限
	SinglePodQps *int32 `json:"singlePodQps"`

	//总QPS
	TotalQps *int32 `json:"totalQps"`

}

// ServerlessWebStatus defines the observed state of ServerlessWeb
type ServerlessWebStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	//该业务实际QPS
	RealQps *int32 `json:"realQps"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ServerlessWeb is the Schema for the serverlesswebs API
type ServerlessWeb struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerlessWebSpec   `json:"spec,omitempty"`
	Status ServerlessWebStatus `json:"status,omitempty"`
}

func (serverlessWeb *ServerlessWeb) String() string {
	var realQps string

	if nil == serverlessWeb.Status.RealQps{
		realQps="nil"
	}else {
		realQps=strconv.Itoa(int(*(serverlessWeb.Status.RealQps)))
	}

	return fmt.Sprintf("Image [%s], Port [%d], SinglePodQps [%d], TotalQps [%d], RealQps [%s]",
		serverlessWeb.Spec.Image,
		*(serverlessWeb.Spec.Port),
		*(serverlessWeb.Spec.SinglePodQps),
		*(serverlessWeb.Spec.TotalQps),
		realQps)
}

//+kubebuilder:object:root=true

// ServerlessWebList contains a list of ServerlessWeb
type ServerlessWebList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerlessWeb `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerlessWeb{}, &ServerlessWebList{})
}
