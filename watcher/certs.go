package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/gookit/slog"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
)

func (c *certs) getEnvCerts() (err error) {
	envcert, exists := os.LookupEnv("SMESH-CA-CERT")
	if !exists {
		return fmt.Errorf("unable to find secrets from environment")
	}
	envkey, exists := os.LookupEnv("SMESH-CA-KEY")
	if !exists {
		return fmt.Errorf("unable to find secrets from environment")
	}
	c.cacert = []byte(envcert)
	c.cakey = []byte(envkey)
	return nil
}

func (c *certs) readCACert() (err error) {
	c.cacert, err = os.ReadFile(*c.folder + "ca.crt")
	if err != nil {
		return err
	}
	return nil
}

func (c *certs) readCAKey() (err error) {
	c.cakey, err = os.ReadFile(*c.folder + "ca.key")
	if err != nil {
		return err
	}
	return nil
}

func (c *certs) writeCACert() (err error) {
	// Public key
	certOut, err := os.Create(*c.folder + "ca.crt")
	if err != nil {
		slog.Error("create ca failed", err)
		return err
	}
	certOut.Write(c.cacert)
	certOut.Close()
	slog.Info("written ca.crt")
	return nil
}

func (c *certs) writeCAKey() (err error) {
	// Public key
	certOut, err := os.Create(*c.folder + "ca.key")
	if err != nil {
		slog.Error("create ca failed", err)
		return err
	}
	certOut.Write(c.cakey)
	certOut.Close()
	slog.Info("written ca.key")
	return nil
}

func (c *certs) writeCert(name string) (err error) {
	// Public key
	certOut, err := os.Create(name + ".crt")
	if err != nil {
		slog.Error("create ca failed", err)
		return err
	}
	certOut.Write(c.cert)
	certOut.Close()
	slog.Info("written ca.crt")
	return nil
}

func (c *certs) writeKey(name string) (err error) {
	// Public key
	certOut, err := os.Create(name + ".key")
	if err != nil {
		slog.Error("create ca failed", err)
		return err
	}
	certOut.Write(c.key)
	certOut.Close()
	slog.Info("written ca.key")
	return nil
}

func (c *certs) generateCA() error {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1653),
		Subject: pkix.Name{
			Organization:  []string{"ORGANIZATION_NAME"},
			Country:       []string{"COUNTRY_CODE"},
			Province:      []string{"PROVINCE"},
			Locality:      []string{"CITY"},
			StreetAddress: []string{"ADDRESS"},
			PostalCode:    []string{"POSTAL_CODE"},
			CommonName:    "42CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := &priv.PublicKey
	ca_b, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, priv)
	if err != nil {
		slog.Error("create ca failed")
		return err
	}

	// Public key
	// certOut, err := os.Create("ca.crt")
	// if err != nil {
	// 	slog.Error("create ca failed", err)
	// 	return err
	// }
	c.cacert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca_b})
	//pem.Encode(certOut, )
	//certOut.Close()
	//slog.Info("written ca.crt")

	// Private key
	// keyOut, err := os.OpenFile("ca.key", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	// if err != nil {
	// 	slog.Error("create ca failed", err)
	// 	return err
	// }
	c.cakey = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	//pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	// keyOut.Close()
	// slog.Info("written ca.key")
	return nil
}

func (c *certs) createCertificate(name, ip string) {
	// Load CA
	tls.X509KeyPair(c.cacert, c.cakey)
	catls, err := tls.X509KeyPair(c.cacert, c.cakey)
	if err != nil {
		panic(err)
	}
	ca, err := x509.ParseCertificate(catls.Certificate[0])
	if err != nil {
		panic(err)
	}
	ipAddress := net.ParseIP(ip)
	// Prepare certificate
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1658),
		Subject: pkix.Name{
			Organization:  []string{"ORGANIZATION_NAME"},
			Country:       []string{"COUNTRY_CODE"},
			Province:      []string{"PROVINCE"},
			Locality:      []string{"CITY"},
			StreetAddress: []string{"ADDRESS"},
			PostalCode:    []string{"POSTAL_CODE"},
			CommonName:    "TEST",
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		DNSNames:     []string{name},
		IPAddresses:  []net.IP{ipAddress},
	}
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := &priv.PublicKey

	// Sign the certificate
	cert_b, err := x509.CreateCertificate(rand.Reader, cert, ca, pub, catls.PrivateKey)
	if err != nil {
		panic(err)
	}
	// Public key
	// certOut, err := os.Create(certificate)
	// if err != nil {
	// 	panic(err)
	// }
	c.cert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert_b})
	// pem.Encode(certOut)
	// certOut.Close()
	// slog.Info(fmt.Sprintf("Written %s", certificate))
	c.key = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	// Private key
	// keyOut, err := os.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	// if err != nil {
	// 	panic(err)
	// }
	// pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	// keyOut.Close()
	// slog.Info(fmt.Sprintf("Written %s", key))

}

func (c *certs) loadSecret(name, namespace string, clientSet *kubernetes.Clientset) error {

	// certificate := fmt.Sprint(name + ".crt")
	// key := fmt.Sprint(name + ".key")
	// certData, err := os.ReadFile(certificate)
	// if err != nil {
	// 	return fmt.Errorf("unable to read certificate %v", err)
	// }
	// keyData, err := os.ReadFile(key)
	// if err != nil {
	// 	return fmt.Errorf("unable to read key %v", err)
	// }
	// caData, err := os.ReadFile("ca.crt")
	// if err != nil {
	// 	return fmt.Errorf("unable to read ca %v", err)
	// }

	secretMap := make(map[string][]byte)

	secretMap["SMESH-CA"] = c.cacert
	secretMap["SMESH-CERT"] = c.cert
	secretMap["SMESH-KEY"] = c.key
	secretMap["KUBE-GATEWAY-TOKEN"] = c.token

	secret := v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-gateway-" + name,
		},
		Data: secretMap,
		Type: v1.SecretTypeOpaque,
	}

	s, err := clientSet.CoreV1().Secrets(namespace).Create(context.TODO(), &secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("unable to create secrets %v", err)
	}
	slog.Info(fmt.Sprintf("Created Secret 🔐 [%s]", s.Name))

	return nil
}
func (c *certs) loadCA(clientSet *kubernetes.Clientset) error {
	secretMap := make(map[string][]byte)

	secretMap["ca-cert"] = c.cacert
	secretMap["ca-key"] = c.cakey
	secret := v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "watcher",
		},
		Data: secretMap,
		Type: v1.SecretTypeOpaque,
	}

	s, err := clientSet.CoreV1().Secrets(v1.NamespaceDefault).Create(context.TODO(), &secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("unable to create secrets %v", err)
	}
	slog.Info(fmt.Sprintf("Created Secret 🔐 [%s]", s.Name))

	return nil

}
