package gateway

import (
	"io"
	"net"

	"github.com/gookit/slog"
)

func (c *Config) Copy_gateway(ingress, egress net.Conn) error {
	// We need to create two loops for parsing what is being sent and what is being recieved
	go func() {
		_, err := io.Copy(egress, ingress)
		if err != nil {
			slog.Printf("Failed copying data to target: %v", err)
		}
	}()
	_, err := io.Copy(ingress, egress)
	if err != nil {
		slog.Printf("Failed copying data from target: %v", err)
	}
	return nil
}
