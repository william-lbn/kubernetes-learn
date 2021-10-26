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

package controllers

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	serverlesswebv1 "serverlessweb/api/v1"
)

const (
	// deployment中的APP标签名
	APP_NAME = "serverlessweb-app"
	// tomcat容器的端口号
	CONTAINER_PORT = 8080
	// 单个POD的CPU资源申请
	CPU_REQUEST = "500m"
	// 单个POD的CPU资源上限
	CPU_LIMIT = "500m"
	// 单个POD的内存资源申请
	MEM_REQUEST = "1024Mi"
	// 单个POD的内存资源上限
	MEM_LIMIT = "1024Mi"
)

// ServerlessWebReconciler reconciles a ServerlessWeb object
type ServerlessWebReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=serverlessweb.com.pml,resources=serverlesswebs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=serverlessweb.com.pml,resources=serverlesswebs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=serverlessweb.com.pml,resources=serverlesswebs/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServerlessWeb object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *ServerlessWebReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx, "serverlessweb", req.NamespacedName)

	log.Info("start reconcile logic")

	//实例化数据结构
	instance := &serverlesswebv1.ServerlessWeb{}

	// 通过客户端工具查询
	err := r.Get(ctx, req.NamespacedName, instance)

	if err != nil {
		// 如果没有实例，就返回空结果，不再调用Reconcile
		if errors.IsNotFound(err) {
			log.Info("instance not found, maybe not created or removed")
			return reconcile.Result{}, nil
		}

		log.Error(err, "get instance error")
		return ctrl.Result{}, err
	}

	log.Info("instance :"+ instance.String())

	deployment := &appsv1.Deployment{}

	err = r.Get(ctx, req.NamespacedName, deployment)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("deployment not exists")

			if *(instance.Spec.TotalQps) < 1 {
				log.Info("totalQps < 1 not need deployment.")
				return ctrl.Result{}, nil
			}

			//先创建service
			if err = createServiceIfNotExists(ctx, r, instance, req); err != nil {
				log.Error(err, "create service error.")
				return ctrl.Result{}, err
			}

			//创建deployment
			if err = createDeployment(ctx, r, instance); err != nil {
				log.Error(err, "create deployment error")
				return ctrl.Result{}, err
			}

			//创建成功更新状态
			if err = updateStatus(ctx, r, instance); err != nil {
				log.Error(err, "update status error.")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil
		} else {
			log.Error(err, "get deployment error.")
			return ctrl.Result{}, nil
		}
	}

	// 根据单QPS和总QPS计算期望的副本数
	expectReplicas := getExpectReplicas(instance)

	// 当前deployment的期望副本数
	realReplicas := *deployment.Spec.Replicas

	log.Info(fmt.Sprintf("expectReplicas [%d], realReplicas [%d]", expectReplicas, realReplicas))

	// 如果相等，就直接返回了
	if expectReplicas == realReplicas {
		log.Info("10. return now")
		return ctrl.Result{}, nil
	}

	// 如果不等，就要调整
	*(deployment.Spec.Replicas) = expectReplicas

	log.Info("update deployment's Replicas")
	// 通过客户端更新deployment
	if err = r.Update(ctx, deployment); err != nil {
		log.Error(err, "update deployment replicas error")

		return ctrl.Result{}, err
	}

	log.Info("update status")

	// 如果更新deployment的Replicas成功，就更新状态
	if err = updateStatus(ctx, r, instance); err != nil {
		log.Error(err, "update status error")

		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

//获取期望的副本数
func getExpectReplicas(serverlessweb *serverlesswebv1.ServerlessWeb) int32 {
	//单个pod的Qps
	singlePodQps := *(serverlessweb.Spec.SinglePodQps)

	//期望的总Qps
	totalQps := *(serverlessweb.Spec.TotalQps)

	//Replicas需要创建的副本数
	replicas := totalQps / singlePodQps

	if totalQps%singlePodQps > 0 {
		replicas++
	}

	return replicas
}

//创建service
func createServiceIfNotExists(ctx context.Context, r *ServerlessWebReconciler, serverlessweb *serverlesswebv1.ServerlessWeb, req ctrl.Request) error {

	log := log.FromContext(ctx, "func", "createService")

	service := &corev1.Service{}

	err := r.Get(ctx, req.NamespacedName, service)

	//查询没有结果，service正常，不需要操作
	if err == nil {
		log.Info("service exists.")
		return nil
	}

	//有错误并且不是NotFound，就直接返回
	if !errors.IsNotFound(err) {
		log.Error(err, "query service error.")
		return err
	}

	//实例化一个数据结构
	service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: serverlessweb.Namespace,
			Name:      serverlessweb.Name,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:     "http",
				Port:     8080,
				NodePort: *serverlessweb.Spec.Port,
			},
			},
			Selector: map[string]string{
				"app": APP_NAME,
			},
			Type: corev1.ServiceTypeNodePort,
		},
	}

	//建立关联后，删除serverlessweb资源时就会将service也删除掉
	log.Info("set reference.")
	if err := controllerutil.SetControllerReference(serverlessweb, service, r.Scheme); err != nil {
		log.Error(err, "SetControllerReference error")
		return err
	}

	//创建service
	log.Info("start create service.")
	if err := r.Create(ctx, service); err != nil {
		log.Error(err, "create service error")
		return err
	}
	log.Info("create service success")

	return nil
}

func createDeployment(ctx context.Context, r *ServerlessWebReconciler, serverlessweb *serverlesswebv1.ServerlessWeb) error {
	log := log.FromContext(ctx, "func", "createDeployment")

	//计算期望的pod数量
	expectReplicas := getExpectReplicas(serverlessweb)

	log.Info(fmt.Sprintf("expectReplicas [%d]", expectReplicas))

	//实例化一个数据结构
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: serverlessweb.Namespace,
			Name:      serverlessweb.Name,
		},
		Spec: appsv1.DeploymentSpec{
			//副本数
			Replicas: pointer.Int32Ptr(expectReplicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": APP_NAME,
				},
			},

			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": APP_NAME,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            APP_NAME,
							Image:           serverlessweb.Spec.Image,
							ImagePullPolicy: "IfNotPresent",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									Protocol:      corev1.ProtocolSCTP,
									ContainerPort: CONTAINER_PORT,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"cpu":    resource.MustParse(CPU_REQUEST),
									"memory": resource.MustParse(MEM_REQUEST),
								},
								Limits: corev1.ResourceList{
									"cpu":    resource.MustParse(CPU_LIMIT),
									"memory": resource.MustParse(MEM_LIMIT),
								},
							},
						},
					},
				},
			},
		},
	}

	//这一步很关键，建立关联后，删除serverlessweb资源时就会将deployment也删除
	log.Info("set reference")
	if err := controllerutil.SetControllerReference(serverlessweb, deployment, r.Scheme); err != nil {
		log.Error(err, "SetControllerReference error")
		return err
	}

	//创建deployment
	log.Info("start create deployment")
	if err := r.Create(ctx, deployment); err != nil {
		log.Error(err, "create deployment error")
		return err
	}
	log.Info("create deployment success.")
	return nil
}

// 更新最新状态
func updateStatus(ctx context.Context, r *ServerlessWebReconciler, serverlessweb *serverlesswebv1.ServerlessWeb) error {
	log := log.FromContext(ctx, "func", "updateStatus")

	//单个pod Qps
	singlePodQps := *(serverlessweb.Spec.SinglePodQps)

	// pod总数
	replicas := getExpectReplicas(serverlessweb)

	if nil == serverlessweb.Status.RealQps {
		serverlessweb.Status.RealQps = new(int32)
	}

	*(serverlessweb.Status.RealQps) = singlePodQps * replicas

	log.Info(fmt.Sprintf("singlePodQps [%d], replicas [%d], realQps[%d]", singlePodQps, replicas, *(serverlessweb.Status.RealQps)))

	if err := r.Update(ctx, serverlessweb); err != nil {
		log.Error(err, "update instance error")
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerlessWebReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&serverlesswebv1.ServerlessWeb{}).
		Complete(r)
}
