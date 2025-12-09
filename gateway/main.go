package main

import (
	"fmt"
	"gateway/pkg/manager"

	"github.com/gookit/slog"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

func main() {
	slog.Info("starting the kube-gateway 🐝")

	slog.Info("Finding existing network sessions")
	n, err := net.Connections("tcp")
	if err != nil {
		slog.Errorf("Unable to find existing connections: %s", err)
	} else {
		for x := range n {
			fmt.Printf("%s %s\n", n[x].Laddr, n[x].Raddr)
			x := manager.Tuple{SourceIP: n[x].Laddr.IP, SourcePort: n[x].Laddr.Port, DestIP: n[x].Raddr.IP, DestPort: n[x].Raddr.Port}
			go func() {
				err = x.Run("eth0", true, 3, 20)
			}()

			if err != nil {
				slog.Errorf("Unable to kill existing TCP sessions: %v", err)
			}
		}
	}
	//https://github.com/Colstuwjx/tcpkill/blob/main/main.go
	slog.Info("finding running processes 🤖")
	p, err := process.Processes()
	for x := range p {
		n, _ := p[x].Name()
		fmt.Printf("Process: %s, pid: %d\n", n, p[x].Pid)
	}
	c, err := manager.Setup()
	if err != nil {
		slog.Fatal(err)
	}
	err = manager.LoadEPF(c)
	if err != nil {
		slog.Fatal(err)
	}
	err = manager.Start(c)
	if err != nil {
		slog.Fatal(err)
	}
}
