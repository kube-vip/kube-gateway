package main

import (
	"flag"
	"fmt"
	"os/user"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/gookit/slog"
)

const (
	enabled        = "kube-gateway.io/enabled"
	encryptGateway = "kube-gateway.io/encrypt"
	enableKTLS     = "kube-gateway.io/ktls"
	aiGateway      = "kube-gateway.io/ai"
)

type certs struct {
	cacert []byte
	cakey  []byte
	key    []byte
	cert   []byte
	folder *string
}

func main() {
	var kubeconfig *string
	u, _ := user.Current()
	if u != nil {
		kubeconfig = flag.String("kubeconfig", u.HomeDir+"/.kube/config", "Path to Kubernetes config")
	}

	var certCollection certs
	slog.Info("Watcher, starting")
	slog.Info("Starting Certicate creation 🔏")
	ca := flag.Bool("ca", false, "Create a CA")
	certName := flag.String("cert", "", "Create a certificate from the CA")
	certCollection.folder = flag.String("certFolder", "", "Create a certificate from the CA")

	certIP := flag.String("ip", "192.168.0.1", "Create a certificate from the CA")
	certSecret := flag.Bool("load", false, "Create a secret in Kubernetes with the certificate")
	loadCA := flag.Bool("loadca", false, "Create a secret in Kubernetes with the certificate")
	watch := flag.Bool("watch", false, "Watch Kubernetes for pods being created and create certs")
	image := flag.String("image", "thebsdbox/kube-gateway:v1", "The image to be used as the gateway")

	flag.Parse()

	if *ca {
		err := certCollection.generateCA()
		if err != nil {
			panic(err)
		}
		err = certCollection.writeCACert()
		if err != nil {
			panic(err)
		}
		err = certCollection.writeCAKey()
		if err != nil {
			panic(err)
		}
	}
	if *loadCA {
		err := certCollection.readCACert()
		if err != nil {
			slog.PanicErr(err)
		}
		err = certCollection.readCAKey()
		if err != nil {
			slog.PanicErr(err)
		}

		c, err := client(*kubeconfig)
		if err != nil {
			slog.PanicErr(err)
		}
		err = certCollection.loadCA(c)
		if err != nil {
			slog.PanicErr(err)
		}
	}
	if *certName != "" {
		certCollection.createCertificate(*certName, *certIP)
		err := certCollection.writeCert(*certName)
		if err != nil {
			panic(err)
		}
		err = certCollection.writeKey(*certName)
		if err != nil {
			panic(err)
		}
		if *certSecret {
			c, err := client(*kubeconfig)
			if err != nil {
				slog.PanicErr(err)
			}
			err = certCollection.loadSecret(*certName, c)
			if err != nil {
				slog.Error("secret", "msg", err)
			}
		}
	}
	if *watch {
		err := certCollection.getEnvCerts()
		if err != nil {
			slog.Warnf("Error reading certificates from env vars [%v]", err)

			err := certCollection.readCACert()
			if err != nil {
				slog.PanicErr(err)
			}
			err = certCollection.readCAKey()
			if err != nil {
				slog.PanicErr(err)
			}
		}
		var c *kubernetes.Clientset
		if kubeconfig == nil {
			c, err = client("")

		} else {
			c, err = client(*kubeconfig)
		}
		if err != nil {
			slog.PanicErr(err)
		}
		certCollection.watcher(c, image)
	}

}

func client(kubeconfigPath string) (*kubernetes.Clientset, error) {
	var kubeconfig *rest.Config

	if kubeconfigPath != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("unable to load kubeconfig from %s: %v", kubeconfigPath, err)
		}
		kubeconfig = config
	} else {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("unable to load in-cluster config: %v", err)
		}
		kubeconfig = config
	}

	// build the client set
	clientSet, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("creating the kubernetes client set - %s", err)
	}
	return clientSet, nil
}

// Actual watcher code

type informerHandler struct {
	clientset *kubernetes.Clientset
	c         *certs
	image     string
}

func (c *certs) watcher(clientSet *kubernetes.Clientset, image *string) error {

	factory := informers.NewSharedInformerFactory(clientSet, 0)

	informer := factory.Core().V1().Pods().Informer()

	_, err := informer.AddEventHandler(&informerHandler{clientset: clientSet, c: c, image: *image})
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

	// Look at all the ephemeral containers added to the pod, if the gateway already exists then don't change the pod
	for x := range newPod.Status.EphemeralContainerStatuses {
		if newPod.Status.EphemeralContainerStatuses[x].Name == "kube-gateway" {
			return
		}
	}
	// Inspect the changes, ensure we have an IP address and the annotation exists
	if newPod.Status.PodIP != "" && newPod.Annotations[enabled] == "" && annotationLookup([]string{aiGateway, encryptGateway}, newPod.Annotations) {

		// 2. Add an ephemeral container to the pod spec.
		podWithEphemeralContainer := i.withProxyContainer(newPod, &i.image)

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

func (i *informerHandler) withProxyContainer(pod *v1.Pod, image *string) *v1.Pod {

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
	// Check for encyption annotation
	if pod.Annotations[encryptGateway] != "" {
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
