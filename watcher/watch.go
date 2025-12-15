package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gookit/slog"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

func (c *certs) watcher(clientSet *kubernetes.Clientset, image *string, imagePull *bool, podCidr *string) error {

	factory := informers.NewSharedInformerFactory(clientSet, 0)

	informer := factory.Core().V1().Pods().Informer()

	_, err := informer.AddEventHandler(&informerHandler{clientset: clientSet, c: c, image: *image, imagePull: *imagePull, podCIDR: *podCidr})
	if err != nil {
		return err
	}
	stop := make(chan struct{}, 2)

	go informer.Run(stop)
	forever := make(chan os.Signal, 1)
	signal.Notify(forever, syscall.SIGINT, syscall.SIGTERM)
	<-forever
	stop <- struct{}{}
	close(forever)
	close(stop)
	return nil
}

func (i *informerHandler) OnUpdate(oldObj, newObj interface{}) {
	newPod := newObj.(*v1.Pod)
	// oldPod := oldObj.(*v1.Pod)

	// Inspect the changes, ensure we have an IP address and the annotation exists
	if newPod.Status.PodIP != "" && newPod.Annotations[enabled] == "" && annotationLookup([]string{aiGateway, encryptGateway, endpoint}, newPod.Annotations) {

		// 2. Add an ephemeral container to the pod spec.
		podWithEphemeralContainer := i.withProxyContainer(newPod, &i.image, i.imagePull)

		// 3. Prepare the patch.
		podJSON, err := json.Marshal(newPod)
		if err != nil {
			panic(err.Error())
		}

		podWithEphemeralContainerJSON, err := json.Marshal(podWithEphemeralContainer)
		if err != nil {
			panic(err.Error())
		}

		patch, err := strategicpatch.CreateTwoWayMergePatch(podJSON, podWithEphemeralContainerJSON, newPod)
		if err != nil {
			panic(err.Error())
		}

		// 4. Apply the patch.

		newPod, err = i.clientset.CoreV1().
			Pods(newPod.Namespace).
			Patch(
				context.TODO(),
				newPod.Name,
				types.StrategicMergePatchType,
				patch,
				metav1.PatchOptions{},
				"ephemeralcontainers",
			)
		if err != nil {
			slog.Error("patching container", "err", err.Error())
		}

		// Update the annotations as we've successfully enabled the proxy
		newPod.Annotations[enabled] = "true"
		_, err = i.clientset.CoreV1().Pods(newPod.Namespace).Update(context.TODO(), newPod, metav1.UpdateOptions{})
		if err != nil {
			slog.Error("updating container", "err", err.Error())
		}

		slog.Info("Ephemeral containers", "name", newPod.Name, "added", len(newPod.Spec.EphemeralContainers), "Annotated", newPod.Annotations[enabled])
	}
}

func (i *informerHandler) OnDelete(obj interface{}) {
	p := obj.(*v1.Pod)
	name := fmt.Sprintf("%s-smesh", p.Name)
	err := i.clientset.CoreV1().Secrets(p.Namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		slog.Errorf("Error deleting secret %v", err)
	} else {
		slog.Infof("Deleted secret 🔏 [%s]", name)
	}

}

func (i *informerHandler) OnAdd(obj interface{}, b bool) {
}

func (i *informerHandler) withProxyContainer(pod *v1.Pod, image *string, forcePull bool) *v1.Pod {

	privileged := true
	secret := pod.Name + "-smesh"
	ec := &v1.EphemeralContainer{
		TargetContainerName: pod.Spec.Containers[0].Name,
		EphemeralContainerCommon: v1.EphemeralContainerCommon{
			Name:  "kube-gateway",
			Image: *image,
			SecurityContext: &v1.SecurityContext{
				Privileged: &privileged, // TODO: Fix permissions
			},
		},
	}

	// Ensure that we always pull the latest image
	if forcePull {
		ec.EphemeralContainerCommon.ImagePullPolicy = v1.PullAlways
	}

	// Check for encyption annotation
	if pod.Annotations[encryptGateway] != "" {
		// Ensure the kube-gateway enables encryption
		ec.EphemeralContainerCommon.Env = append(ec.EphemeralContainerCommon.Env, v1.EnvVar{Name: "ENCRYPT", Value: "TRUE"})

		// Create certificates and then a Kubernetes secret
		i.c.createCertificate(pod.Name, pod.Status.PodIP)

		err := i.c.loadSecret(pod.Name, i.clientset)
		if err != nil {
			slog.Error(err)
		}
		ec.EnvFrom = append(ec.EnvFrom, v1.EnvFromSource{
			SecretRef: &v1.SecretEnvSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: secret,
				},
				Optional: nil,
			},
		})

		// If we're wanting to offload TLS to the kernel
		if pod.Annotations[enableKTLS] != "" {
			ec.EphemeralContainerCommon.Env = append(ec.EphemeralContainerCommon.Env, v1.EnvVar{Name: "KTLS", Value: "TRUE"})
		}

	}

	// Enable AI gateway
	if pod.Annotations[aiGateway] != "" {
		ec.EphemeralContainerCommon.Env = append(ec.EphemeralContainerCommon.Env, v1.EnvVar{Name: "AI", Value: "TRUE"})
		// override the AI model
		if pod.Annotations[aiModel] != "" {
			ec.EphemeralContainerCommon.Env = append(ec.EphemeralContainerCommon.Env, v1.EnvVar{Name: "MODEL", Value: pod.Annotations[aiModel]})
		}
	}

	// Enable the debug mode
	if pod.Annotations[podcidr] != "" {
		ec.EphemeralContainerCommon.Env = append(ec.EphemeralContainerCommon.Env, v1.EnvVar{Name: "PODCIDR", Value: pod.Annotations[podcidr]})
	} else {
		ec.EphemeralContainerCommon.Env = append(ec.EphemeralContainerCommon.Env, v1.EnvVar{Name: "PODCIDR", Value: i.podCIDR})
	}

	// Enable the debug mode
	if pod.Annotations[debug] != "" {
		ec.EphemeralContainerCommon.Env = append(ec.EphemeralContainerCommon.Env, v1.EnvVar{Name: "DEBUG", Value: "TRUE"})
	}

	// Enable the debug mode
	if pod.Annotations[debug] != "" {
		ec.EphemeralContainerCommon.Env = append(ec.EphemeralContainerCommon.Env, v1.EnvVar{Name: "DEBUG", Value: "TRUE"})
	}

	// Set the pod to have an enabled annotation
	pod.Annotations[enabled] = "true"

	copied := pod.DeepCopy()
	copied.Spec.EphemeralContainers = append(copied.Spec.EphemeralContainers, *ec)
	copied.Spec.ShareProcessNamespace = &privileged
	return copied
}

// Loop through the annotations to see if any of them are set
func annotationLookup(annotation []string, annotations map[string]string) (found bool) {
	for x := range annotation {
		if _, ok := annotations[annotation[x]]; ok {
			found = true
		}
	}
	return
}
