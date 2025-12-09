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
