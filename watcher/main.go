package main

import (
	"context"
	"flag"
	"fmt"
	"os/user"

	authenticationv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/gookit/slog"
)

// Annotations that are applied to a pod, that the watcher will translate to an environment variable for the kube-gateway pod
const (
	// Configuration
	debug   = "kube-gateway.io/debug"
	podcidr = "kube-gateway.io/podcidr"

	// Be a simple endpoint to a gateway
	endpoint = "kube-gateway.io/endpoint"

	// Encryption annotations
	encryptGateway = "kube-gateway.io/encrypt"
	enableKTLS     = "kube-gateway.io/ktls"

	// AI annotations
	aiGateway = "kube-gateway.io/ai"
	aiModel   = "kube-gateway.io/ai-model"

	// Network flush annotation
	netflush = "kube-gateway.io/netflush"

	// This should be set once the gateway has been enabled on a pod
	enabled = "kube-gateway.io/enabled"
)

type certs struct {
	cacert []byte
	cakey  []byte
	key    []byte
	cert   []byte
	token  []byte
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
	podcidr := flag.String("podcidr", "10.0.0.0/16", "Set the PodCIDR for capturing traffic")

	certIP := flag.String("ip", "192.168.0.1", "Create a certificate from the CA")
	certSecret := flag.Bool("load", false, "Create a secret in Kubernetes with the certificate")
	loadCA := flag.Bool("loadca", false, "Create a secret in Kubernetes with the certificate")
	watch := flag.Bool("watch", false, "Watch Kubernetes for pods being created and create certs")
	image := flag.String("image", "thebsdbox/kube-gateway:v1", "The image to be used as the gateway")
	imagePull := flag.Bool("forcePull", false, "ensure that the gatewway image is always pulled")

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
			err = certCollection.loadSecret(*certName, "kube-gateway", c)
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
		certCollection.watcher(c, image, imagePull, podcidr)
	}

}

func checkSecretExists(c *kubernetes.Clientset, namespace string) []byte {
	var s *v1.Secret
	var err error
	s, err = c.CoreV1().Secrets(namespace).Get(context.TODO(), "kube-gateway", metav1.GetOptions{})

	if err != nil {
		slog.Errorf("finding secret %v", err)

		// lets create the correct settings
		account := v1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "kube-gateway", Namespace: namespace}}

		_, err = c.CoreV1().ServiceAccounts(namespace).Create(context.TODO(), &account, metav1.CreateOptions{})
		if err != nil {
			slog.Errorf("creating service account %v", err)
		}
		clusterRoleBinding := &rbacv1.ClusterRoleBinding{
			TypeMeta: metav1.TypeMeta{APIVersion: rbacv1.SchemeGroupVersion.String(), Kind: "ClusterRoleBinding"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "system:kube-gateway-binding",
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     "system:kube-gateway-role",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "kube-gateway",
					Namespace: namespace,
				},
			},
		}
		_, err = c.RbacV1().ClusterRoleBindings().Create(context.TODO(), clusterRoleBinding, metav1.CreateOptions{})
		if err != nil {
			slog.Errorf("creating role binding %v", err)
		}
		expirationSeconds := int64(60 * 60 * 24 * 365)

		t, err := c.CoreV1().ServiceAccounts(namespace).CreateToken(context.TODO(), "kube-gateway", &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &expirationSeconds}}, metav1.CreateOptions{})
		if err != nil {
			slog.Errorf("creating secret %v", err)
		}
		secretMap := make(map[string][]byte)

		secretMap["KUBE-GATEWAY-TOKEN"] = []byte(t.Status.Token)
		secret := v1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "kube-gateway",
			},
			Data: secretMap,
			Type: v1.SecretTypeOpaque,
		}

		s, err = c.CoreV1().Secrets(namespace).Create(context.TODO(), &secret, metav1.CreateOptions{})
		if err != nil {
			slog.Errorf("unable to create secrets %v", err)
		}
		slog.Info(fmt.Sprintf("Created Secret 🔐 [%s]", s.Name))
	} else {
		slog.Info("Existing service account exists")
	}

	return s.Data["KUBE-GATEWAY-TOKEN"]
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
