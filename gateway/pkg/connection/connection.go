package connection

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/cilium/ebpf"
	"github.com/gookit/slog"
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

	Socks *ebpf.Map

	ProxyFunc func(string) string

	// Environment Variables
	Tunnel  bool // Running as a tunnel compared to a sidecar
	Encrypt bool // Load certificates as traffic is encrypted
	KTLS    bool // Enable Kernel TLS
	Flush   bool // Find existing network connections and terminate them
	AI      bool // Workload is going to be AI
}

func (c *Config) StartInternalListener() net.Listener {
	proxyAddr := fmt.Sprintf("%s:%d", c.Address, c.ProxyPort)
	listener, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		slog.Fatalf("Failed to start proxy server: %v", err)
	}
	slog.Infof("[pid: %d] %s", os.Getpid(), proxyAddr)
	return listener
}

func (c *Config) StartExternalListener() net.Listener {
	proxyAddr := fmt.Sprintf("0.0.0.0:%d", c.ClusterPort)
	listener, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		slog.Fatalf("Failed to start proxy server: %v", err)
	}
	slog.Infof("[pid: %d] %s", os.Getpid(), proxyAddr)
	return listener
}

// Blocking function
func (c *Config) StartListeners(listener net.Listener, internal bool) {
	for {
		conn, err := listener.Accept()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			slog.Printf("Failed to accept connection: %v", err)
			continue
		} else {
			if conn != nil {
				if internal {
					slog.Printf("internal %s -> %s", conn.RemoteAddr().String(), conn.LocalAddr().String())
					if c.KTLS {
						go c.internalkTLSProxy(conn)
					} else {
						go c.internalTLSProxy(conn)
					}
				} else {
					slog.Printf("external %s -> %s", conn.RemoteAddr().String(), conn.LocalAddr().String())
					go c.handleExternalConnection(conn)
				}
			}
		}
	}
}

// Unencrypted external connection
func (c *Config) handleExternalConnection(conn net.Conn) {
	defer conn.Close()

	tmp := make([]byte, 256)
	n, err := conn.Read(tmp)
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
	if err != nil {
		slog.Printf("Failed to connect to original destination[%s]: %v", string(tmp), err)
		return
	}
	defer targetConn.Close()
	conn.Write([]byte{'Y'}) // Send a response to kickstart the comms

	slog.Printf("%s -> %s", conn.RemoteAddr(), targetConn.RemoteAddr())

	// The following code creates two data transfer channels:
	// - From the client to the target server (handled by a separate goroutine).
	// - From the target server to the client (handled by the main goroutine).
	go func() {
		_, err = io.Copy(targetConn, conn)
		if err != nil {
			slog.Printf("Failed copying data to target: %v", err)
		}
	}()
	_, err = io.Copy(conn, targetConn)
	if err != nil {
		slog.Printf("Failed copying data from target: %v", err)
	}
}
