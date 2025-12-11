package connection

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/gookit/slog"
)

func (c *Config) StartExternalTLSListener() net.Listener {
	proxyAddr := fmt.Sprintf("0.0.0.0:%d", c.ClusterTLSPort)

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(c.Certificates.ca) {
		log.Fatalf("could not append CA")
	}
	certificate, err := tls.X509KeyPair(c.Certificates.cert, c.Certificates.key)

	if err != nil {
		log.Fatalf("could not load certificate: %v", err)
	}

	config := &tls.Config{
		ClientCAs:    caCertPool,
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   tls.VerifyClientCertIfGiven,
	} //<-- this is the key

	listener, err := tls.Listen("tcp", proxyAddr, config)

	// listener, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		slog.Fatalf("Failed to start proxy server: %v", err)
	}
	slog.Infof("[pid: %d] %s", os.Getpid(), proxyAddr)
	return listener
}

// Blocking function
func (c *Config) StartTLSListener(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) { // Don't print closing connection error, just continue to the next
				slog.Printf("Failed to accept connection: %v", err)
			}
			continue
		}

		slog.Printf(" %s -> %s", conn.RemoteAddr().String(), conn.LocalAddr().String())
		go c.handleTLSExternalConnection(conn)

	}
}


func (c *Config) createTLSProxy(destAddr string) (net.Conn, error) {
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(c.Certificates.ca) {
		log.Fatalf("could not append CA")
	}
	certificate, err := tls.X509KeyPair(c.Certificates.cert, c.Certificates.key)
	if err != nil {
		log.Fatalf("could not load certificate: %v", err)
	}

	config := &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   tls.VerifyClientCertIfGiven,
	} //<-- this is the key

	endpoint := fmt.Sprintf("%s:%d", destAddr, c.ClusterTLSPort)
	if c.ClusterAddress != "" {
		endpoint = fmt.Sprintf("%s:%d", c.ClusterAddress, c.ClusterPort)
	}
	if c.Tunnel {
		endpoint = fmt.Sprintf("%s:%d", c.ProxyFunc(destAddr), c.ClusterPort)
	}

	// Set a timeout, mainly because connections can occur to pods that aren't ready
	d := net.Dialer{Timeout: time.Second * 3}
	targetConn, err := tls.DialWithDialer(&d, "tcp", endpoint, config)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to destination TLS proxy: %v", err)
	}
	return targetConn, nil
}

// Unencrypted external connection
func (c *Config) handleTLSExternalConnection(conn net.Conn) {
	defer conn.Close()
	var tConn *tls.Conn = conn.(*tls.Conn)

	tmp := make([]byte, 256)
	n, err := tConn.Read(tmp)
	if err != nil {
		slog.Print(err)
	}

	remoteAddress := string(tmp[:n])

	if remoteAddress == fmt.Sprintf("%s:%d", c.Address, c.ProxyPort) {
		slog.Printf("Potential loopback")
		return
	}

	// Check that the original destination address is reachable from the proxy
	targetConn, err := net.DialTimeout("tcp", remoteAddress, 5*time.Second)
	//targetConn, err := tls.Dial("tcp", remoteAddress, config)
	if err != nil {
		slog.Printf("Failed to connect to original destination[%s]: %v", string(tmp), err)
		return
	}
	defer targetConn.Close()
	tConn.Write([]byte{'Y'}) // Send a response to kickstart the comms

	slog.Printf("%s -> %s", conn.RemoteAddr(), targetConn.RemoteAddr())

	// The following code creates two data transfer channels:
	// - From the client to the target server (handled by a separate goroutine).
	// - From the target server to the client (handled by the main goroutine).
	go func() {
		_, err = io.Copy(targetConn, tConn)
		if err != nil {
			slog.Printf("Failed copying data to target: %v", err)
		}
	}()
	_, err = io.Copy(tConn, targetConn)
	if err != nil {
		slog.Printf("Failed copying data from target: %v", err)
	}
}
