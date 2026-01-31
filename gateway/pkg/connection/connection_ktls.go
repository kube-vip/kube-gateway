package connection

import (
	"crypto/x509"
	"errors"
	"fmt"
	"gateway/pkg/gateway"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"time"

	"gitlab.com/go-extension/tls"
)

func (c *Config) StartExternalkTLSListener() net.Listener {
	proxyAddr := fmt.Sprintf("0.0.0.0:%d", c.ClusterTLSPort)

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(c.Certificates.ca) {
		panic("could not append CA")
	}
	certificate, err := tls.X509KeyPair(c.Certificates.cert, c.Certificates.key)

	if err != nil {
		panic(fmt.Sprintf("could not load certificate: %v", err))
	}

	config := &tls.Config{
		ClientCAs:    caCertPool,
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   tls.VerifyClientCertIfGiven,
		KernelTX:     true,
		KernelRX:     true,
	} //<-- this is the key

	listener, err := tls.Listen("tcp", proxyAddr, config)

	// listener, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		panic(fmt.Sprintf("Failed to start proxy server: %v", err))
	}
	slog.Info("external KTLS listener", "pid", os.Getpid(), "proxyaddr", proxyAddr)
	return listener
}

// Blocking function
func (c *Config) StartkTLSListener(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) { // Don't print closing connection error, just continue to the next
				slog.Error("accept connection", "err", err)
			}
			continue
		}

		slog.Info("accepted connection", "remote", conn.RemoteAddr().String(), "local", conn.LocalAddr().String())
		go c.handlekTLSExternalConnection(conn)

	}
}

// HTTP proxy request handler
func (c *Config) internalkTLSProxy(conn net.Conn) {
	defer conn.Close()
	// Get original destination address
	destAddr, destPort, err := c.findTargetFromConnection(conn)
	if err != nil {
		return
	}
	targetDestination := fmt.Sprintf("%s:%d", destAddr, destPort)
	var targetConn net.Conn
	var endpoint string
	// Send traffic to endpoint gateway
	if c.Certificates != nil {

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
			KernelTX:     true,
			KernelRX:     true,
		} //<-- this is the key

		endpoint = fmt.Sprintf("%s:%d", destAddr, c.ClusterTLSPort)
		if c.ClusterAddress != "" {
			endpoint = fmt.Sprintf("%s:%d", c.ClusterAddress, c.ClusterPort)
		}
		if c.Tunnel {
			endpoint = fmt.Sprintf("%s:%d", c.ProxyFunc(destAddr), c.ClusterPort)
		}

		// Set a timeout, mainly because connections can occur to pods that aren't ready
		d := net.Dialer{Timeout: time.Second * 3}
		targetConn, err = tls.DialWithDialer(&d, "tcp", endpoint, config)
		if err != nil {
			slog.Error("connecting to destination TLS proxy", "err", err)
			return
		}
	} else {
		endpoint = fmt.Sprintf("%s:%d", destAddr, c.ClusterPort)
		if c.ClusterAddress != "" {
			endpoint = fmt.Sprintf("%s:%d", c.ClusterAddress, c.ClusterPort)
		}
		if c.Tunnel {
			endpoint = fmt.Sprintf("%s:%d", c.ProxyFunc(destAddr), c.ClusterPort)
		}
		// Check that the original destination address is reachable from the proxy
		targetConn, err = net.DialTimeout("tcp", endpoint, 5*time.Second)
		if err != nil {
			slog.Error("connecting to original destination", "err", err)
			return
		}
	}
	defer targetConn.Close()

	slog.Info("connecting", "proxy", endpoint, "origin", targetDestination)
	//log.Printf("Internal proxy sending original destination: %s\n", targetDestination)
	_, err = targetConn.Write([]byte(targetDestination))
	if err != nil {
		slog.Error("network write", "endpoint", endpoint, "err", err)
	}

	tmp := make([]byte, 256)

	// Ideally we wait here until our remote endpoint has recieved the targetDestination
	targetConn.Read(tmp)

	//log.Printf("Internal connection from %s to %s\n", conn.RemoteAddr(), targetConn.RemoteAddr())

	// The following code creates two data transfer channels:
	// - From the client to the target server (handled by a separate goroutine).
	// - From the target server to the client (handled by the main goroutine).
	go func() {
		_, err = io.Copy(targetConn, conn)
		if err != nil {
			slog.Error("copying data to target", "err", err)
		}
	}()
	_, err = io.Copy(conn, targetConn)
	if err != nil {
		slog.Error("copying data from target", "err", err)
	}
}

// Unencrypted external connection
func (c *Config) handlekTLSExternalConnection(conn net.Conn) {
	defer conn.Close()
	var tConn *tls.Conn = conn.(*tls.Conn)

	tmp := make([]byte, 256)
	n, err := tConn.Read(tmp)
	if err != nil {
		slog.Error("connection read", "err", err)
	}

	remoteAddress := string(tmp[:n])

	if remoteAddress == fmt.Sprintf("%s:%d", c.Address, c.ProxyPort) {
		slog.Error("Potential loopback", "remoteAdd", remoteAddress)
		return
	}

	// Check that the original destination address is reachable from the proxy
	targetConn, err := net.DialTimeout("tcp", remoteAddress, 5*time.Second)
	//targetConn, err := tls.Dial("tcp", remoteAddress, config)
	if err != nil {
		slog.Error("connection", "original/local addr", string(tmp), "err", err)
		return
	}
	defer targetConn.Close()
	tConn.Write([]byte{'Y'}) // Send a response to kickstart the comms

	slog.Info("connection", "remote", conn.RemoteAddr(), "target", targetConn.RemoteAddr())

	// The following code creates two data transfer channels:
	// - From the client to the target server (handled by a separate goroutine).
	// - From the target server to the client (handled by the main goroutine).
	gateway.Copy_gateway(targetConn, tConn, c.AITransaction)
}
