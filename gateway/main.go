package main

import (
	"gateway/pkg/manager"
	"log/slog"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

func main() {
	slog.Info("starting the kube-gateway 🐝")
	c, err := manager.Setup()
	if err != nil {
		panic(err)
	}
	slog.Info("watching for pods", "CIDR", c.PodCIDR)

	slog.Info("Finding existing network sessions")
	n, err := net.Connections("tcp")
	if err != nil {
		slog.Error("finding existing connections", "err", err)
	} else {
		for x := range n {
			//fmt.Printf("Flushing: %t, source: %s / destination:%s\n", c.Flush, n[x].Laddr, n[x].Raddr)
			if c.Flush {
				if n[x].Laddr.IP != "::" || n[x].Raddr.IP != "::" {
					x := manager.Tuple{SourceIP: n[x].Laddr.IP, SourcePort: n[x].Laddr.Port, DestIP: n[x].Raddr.IP, DestPort: n[x].Raddr.Port}
					go func() {
						err = x.Run("eth0", true, 3, 20)
					}()

					if err != nil {
						slog.Error("killing existing TCP sessions", "err", err)
					}
				}
			}
		}
	}
	slog.Info("finding running processes 🤖")
	p, err := process.Processes()
	for x := range p {
		n, _ := p[x].Name()
		slog.Info("process found", "process", n, "pid", p[x].Pid)
		c.Pids = append(c.Pids, uint32(p[x].Pid))
	}
	err = manager.LoadEPF(c)
	if err != nil {
		panic(err) // TODO: handle better
	}

	err = manager.Start(c)
	if err != nil {
		panic(err) // TODO: handle better
	}
}
