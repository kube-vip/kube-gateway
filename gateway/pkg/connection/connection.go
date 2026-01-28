package connection

import (
	"errors"
	"fmt"
	"gateway/pkg/gateway"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/cilium/ebpf"
)

type Config struct {
	ProxyPort      int
	ClusterPort    int
	ClusterTLSPort int
	Address        string
	ClusterAddress string // For Debug purposes
	CgroupOverride string // For Debug purposes

	PodCIDR      string
	Certificates *Certs
	Token        []byte

	Socks *ebpf.Map

	ProxyFunc func(string) string

	// Environment Variables
	Endpoint bool // Run as a simple endoint
	Tunnel   bool // Running as a tunnel compared to a sidecar
	Encrypt  bool // Load certificates as traffic is encrypted
	KTLS     bool // Enable Kernel TLS
	Flush    bool // Find existing network connections and terminate them
	AI       bool // Workload is going to be AI

	// Gateway
	AITransaction *gateway.AITransaction

	Pids []uint32
}

func (c *Config) CreateInternalListener() (net.Listener, error) {
	proxyAddr := fmt.Sprintf("%s:%d", c.Address, c.ProxyPort)
	listener, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		return nil, err
	}
	slog.Info("listener", "type", "internal", "pid", os.Getpid(), "addr", proxyAddr)
	return listener, nil
}

func (c *Config) CreateExternalListener() (net.Listener, error) {
	proxyAddr := fmt.Sprintf("0.0.0.0:%d", c.ClusterPort)
	listener, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		return nil, err
	}
	slog.Info("listener", "type", "external", "pid", os.Getpid(), "addr", proxyAddr)
	return listener, nil
}

// Blocking function
func (c *Config) StartListeners(listener net.Listener, internal bool) {
	for {
		conn, err := listener.Accept()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			slog.Info("accept connection", "err", err)
			continue
		} else {
			if conn != nil {
				if internal {
					slog.Info("internal connection", "remote", conn.RemoteAddr().String(), "local", conn.LocalAddr().String())
					if c.KTLS {
						go c.internalkTLSProxy(conn)
					} else {
						if c.AI {
							go c.internalProxy(conn, c.AITransaction.Http_gateway)
						} else {
							go c.internalProxy(conn, gateway.Copy_gateway)
						}
					}
				} else {
					slog.Info("external connection", "remote", conn.RemoteAddr().String(), "local", conn.LocalAddr().String())
					go c.handleExternalConnection(conn)
				}
			}
		}
	}
}

// HTTP proxy request handler
func (c *Config) internalProxy(conn net.Conn, gatewayFunc func(net.Conn, net.Conn) error) {
	defer conn.Close()
	// Get original destination address
	destAddr, destPort, err := c.findTargetFromConnection(conn)
	if err != nil {
		return
	}
	targetDestination := fmt.Sprintf("%s:%d", destAddr, destPort)
	var targetConn net.Conn
	// Send traffic to endpoint gateway
	if c.Certificates != nil {
		targetConn, err = c.createTLSProxy(destAddr)
		if err != nil {
			slog.Error("proxy create", "err", err)
			return
		}
		slog.Info("proxy (TLS)", "endpoint", targetConn.RemoteAddr().String())

	} else {
		targetConn, err = c.createProxy(destAddr)
		if err != nil {
			panic(err)
		}
		slog.Info("proxy", "endpoint", targetConn.RemoteAddr().String())

	}
	defer targetConn.Close()

	//log.Printf("Internal proxy sending original destination: %s\n", targetDestination)
	_, err = targetConn.Write([]byte(targetDestination))
	if err != nil {
		slog.Error("destination write", "err", err)
	}

	tmp := make([]byte, 256)

	// Ideally we wait here until our remote endpoint has recieved the targetDestination
	targetConn.Read(tmp)

	//log.Printf("Internal connection from %s to %s\n", conn.RemoteAddr(), targetConn.RemoteAddr())

	// The following code creates two data transfer channels:
	// - From the client to the target server (handled by a separate goroutine).
	// - From the target server to the client (handled by the main goroutine).
	err = gatewayFunc(conn, targetConn)
	if err != nil {
		slog.Error("date write", "err", err)
	}
}

func (c *Config) createProxy(destAddr string) (net.Conn, error) {
	endpoint := fmt.Sprintf("%s:%d", destAddr, c.ClusterPort)
	if c.ClusterAddress != "" {
		endpoint = fmt.Sprintf("%s:%d", c.ClusterAddress, c.ClusterPort)
	}
	if c.Tunnel {
		endpoint = fmt.Sprintf("%s:%d", c.ProxyFunc(destAddr), c.ClusterPort)
	}
	// Check that the original destination address is reachable from the proxy
	targetConn, err := net.DialTimeout("tcp", endpoint, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to original destination: %v", err)
	}
	return targetConn, nil
}

// Unencrypted external connection
func (c *Config) handleExternalConnection(conn net.Conn) {
	defer conn.Close()

	tmp := make([]byte, 256)
	n, err := conn.Read(tmp)
	if err != nil {
		slog.Info("external read", "err", err)
	}
	remoteAddress := string(tmp[:n])

	if remoteAddress == fmt.Sprintf("%s:%d", c.Address, c.ProxyPort) {
		slog.Error("Potential loopback")
		return
	}

	// Check that the original destination address is reachable from the proxy
	targetConn, err := net.DialTimeout("tcp", remoteAddress, 5*time.Second)
	if err != nil {
		slog.Error("connection", "destination", string(tmp), "err", err)
		return
	}
	defer targetConn.Close()
	conn.Write([]byte{'Y'}) // Send a response to kickstart the comms

	slog.Info("connection", "remote", conn.RemoteAddr(), "target", targetConn.RemoteAddr())

	// The following code creates two data transfer channels:
	// - From the client to the target server (handled by a separate goroutine).
	// - From the target server to the client (handled by the main goroutine).
	gateway.Copy_gateway(targetConn, conn)
}
