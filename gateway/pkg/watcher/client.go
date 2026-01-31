package watcher

import (
	"fmt"
	"gateway/pkg/gateway"
	"log/slog"
	"net"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	certutil "k8s.io/client-go/util/cert"
)

type Watch struct {
	token         string
	tokenFile     string
	rootCAFile    string
	namespaceFile string
	config        *gateway.AITransaction
	podname       string
	namespace     string
	configMapName string
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
		slog.Error("expected to load root CA config", "from", w.rootCAFile, "got", err)
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
		config.BearerToken = w.token // Override the token
	}

	kubeconfig = config

	// build the client set
	clientSet, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("creating the kubernetes client set - %s", err)
	}
	return clientSet, nil
}

func NewWatcher(pid int, token string, config *gateway.AITransaction) *Watch {
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
		config:        config,
		podname:       os.Getenv("POD_NAME"),
		namespace:     os.Getenv("POD_NAMESPACE"),
	}
}
