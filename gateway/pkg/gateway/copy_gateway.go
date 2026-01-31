package gateway

import (
	"io"
	"log/slog"
	"net"
)

func Copy_gateway(ingress, egress net.Conn, c *AITransaction) error {
	// We need to create two loops for parsing what is being sent and what is being recieved
	go func() {
		_, err := io.Copy(egress, ingress)
		if err != nil {
			slog.Error("copying data to target", "err", err)
		}
	}()
	_, err := io.Copy(ingress, egress)
	if err != nil {
		slog.Error("copying data from target", "err", err)
	}
	return nil //TODO: this is unused, but matched signature header
}
