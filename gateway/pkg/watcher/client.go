package watcher

import (
	"encoding/json"
	"fmt"
	"gateway/pkg/gateway"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog/v2"
)

type Watch struct {
	token         string
	tokenFile     string
	rootCAFile    string
	namespaceFile string
}

func (w *Watch) InClusterConfig() (*rest.Config, error) {

	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if len(host) == 0 || len(port) == 0 {
		return nil, rest.ErrNotInCluster
	}

	token, err := os.ReadFile(w.tokenFile)
	if err != nil {
		return nil, err
	}

	tlsClientConfig := rest.TLSClientConfig{}

	if _, err := certutil.NewPool(w.rootCAFile); err != nil {
		//nolint:logcheck // The decision to log this instead of returning an error goes back to ~2016. It's part of the client-go API now, so not changing it just to support contextual logging.
		klog.Errorf("Expected to load root CA config from %s, but got err: %v", w.rootCAFile, err)
	} else {
		tlsClientConfig.CAFile = w.rootCAFile
	}

	return &rest.Config{
		// TODO: switch to using cluster DNS.
		Host:            "https://" + net.JoinHostPort(host, port),
		TLSClientConfig: tlsClientConfig,
		BearerToken:     string(token),
		//BearerTokenFile: w.tokenFile,
		//Username: "kube-gateway",
	}, nil
}

func (w *Watch) client() (*kubernetes.Clientset, error) {
	var kubeconfig *rest.Config
	config, err := w.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to load in-cluster config: %v", err)
	}

	if w.token != "" {
		slog.Info("kubernetes token", "Overriding", true)
		config.BearerToken = w.token // Override the token
		fmt.Println(w.token)
	}

	kubeconfig = config

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
	config    *gateway.AIConfig
	namespace []byte
	podName   string
}

func NewWatcher(pid int, token string) *Watch {
	const (
		nameSpaceFile = "namespace"
		tokenFile     = "token"
		rootCAFile    = "ca.crt"
	)

	return &Watch{
		token:         token,
		namespaceFile: fmt.Sprintf("/proc/%d/root/var/run/secrets/kubernetes.io/serviceaccount/%s", pid, nameSpaceFile),
		tokenFile:     fmt.Sprintf("/proc/%d/root/var/run/secrets/kubernetes.io/serviceaccount/%s", pid, tokenFile),
		rootCAFile:    fmt.Sprintf("/proc/%d/root/var/run/secrets/kubernetes.io/serviceaccount/%s", pid, rootCAFile),
	}
}

func (w *Watch) Start(config *gateway.AIConfig) (err error) {
	clientSet, err := w.client()
	if err != nil {
		return err
	}
	handler := informerHandler{clientset: clientSet, config: config}

	// Retrieve the namespace
	handler.namespace, err = os.ReadFile(w.namespaceFile)
	if err != nil {
		panic(err)
	}

	// Retrieve the pod name
	handler.podName, err = os.Hostname()
	if err != nil {
		panic(err)
	}
	slog.Info("starting watcher", "name", handler.podName, "namespace", handler.namespace)

	factory := informers.NewSharedInformerFactory(clientSet, 0)

	informer := factory.Core().V1().ConfigMaps().Informer()

	_, err = informer.AddEventHandler(&handler)
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
	updatedConfigMap := newObj.(*v1.ConfigMap)
	if updatedConfigMap.Name == i.podName && updatedConfigMap.Namespace == string(i.namespace) {
		data := updatedConfigMap.Data["config"]
		err := json.Unmarshal([]byte(data), i.config)
		if err != nil {
			slog.Error("unable to read JSON from configMap", "err", err)
		}
		fmt.Println(data)
	}
	// oldPod := oldObj.(*v1.Pod)

}

func (i *informerHandler) OnDelete(obj interface{}) {
}

func (i *informerHandler) OnAdd(obj interface{}, b bool) {
	configmap := obj.(*v1.ConfigMap)
	fmt.Println(configmap.Name)

}
