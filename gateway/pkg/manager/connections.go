package manager

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/gookit/slog"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	//"github.com/gopacket/gopacket/pcap"
	//gopcap "github.com/packetcap/go-pcap"
)

type Tuple struct {
	SourceIP   string
	SourcePort uint32
	DestIP     string
	DestPort   uint32
}

func sendRST(srcMac, dstMac net.HardwareAddr, srcIP, dstIP net.IP, srcPort, dstPort layers.TCPPort, seq uint32, handle *pcap.Handle) error {
	log.Printf("send %v:%v > %v:%v [RST] seq %v", srcIP.String(), srcPort.String(), dstIP.String(), dstPort.String(), seq)

	eth := layers.Ethernet{
		SrcMAC:       srcMac,
		DstMAC:       dstMac,
		EthernetType: layers.EthernetTypeIPv4,
	}

	iPv4 := layers.IPv4{
		SrcIP:    srcIP,
		DstIP:    dstIP,
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
	}

	tcp := layers.TCP{
		SrcPort: srcPort,
		DstPort: dstPort,
		Seq:     seq,
		RST:     true,
	}

	if err := tcp.SetNetworkLayerForChecksum(&iPv4); err != nil {
		return err
	}

	buffer := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	if err := gopacket.SerializeLayers(buffer, options, &eth, &iPv4, &tcp); err != nil {
		return err
	}
	err := handle.WritePacketData(buffer.Bytes())
	if err != nil {
		return err
	}

	return nil
}

func (t *Tuple) Run(iface string, prom bool, count, timeout int) error {
	fmt.Printf("tcpkill listen on %v\n", iface)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	var handle *pcap.Handle
	var err error

	// snaplen and timeout hard-code
	if handle, err = pcap.OpenLive(iface, int32(65535), prom, -1*time.Second); err != nil {
		return err
	}

	defer handle.Close()
	//bpffilter := fmt.Sprintf("src host %s and port %d and dst host %s and port %d", t.SourceIP, t.SourcePort, t.DestIP, t.DestPort)
	//fmt.Fprintf(os.Stderr, "Using BPF filter %q\n", bpffilter)
	//	if err = handle.SetBPFFilter(bpffilter); err != nil {
	//		return fmt.Errorf("set BPF filter error: %v", err)
	//	}

	packetSource := gopacket.NewPacketSource(
		handle,
		handle.LinkType(),
	)
	var ingress, egress bool

	for packet := range packetSource.Packets() {

		select {
		case <-ctx.Done():
			slog.Info("Timed out")
			return nil
		default:

			ethLayer := packet.Layer(layers.LayerTypeEthernet)
			if ethLayer == nil {
				continue
			}
			eth := ethLayer.(*layers.Ethernet)

			ipv4Layer := packet.Layer(layers.LayerTypeIPv4)
			if ipv4Layer == nil {
				continue
			}
			ip := ipv4Layer.(*layers.IPv4)

			tcpLayer := packet.Layer(layers.LayerTypeTCP)
			if tcpLayer == nil {
				continue
			}
			tcp := tcpLayer.(*layers.TCP)

			if tcp.SYN || tcp.FIN || tcp.RST {
				continue
			}
			//fmt.Printf("%s %d %s %d == %s %d %s %d\n", ip.SrcIP.String(), tcp.SrcPort, ip.DstIP.String(), tcp.DstPort, t.SourceIP, t.SourcePort, t.DestIP, t.DestPort)

			// if t.SourceIP == ip.SrcIP.String() &&
			// 	t.SourcePort == uint32(tcp.SrcPort) &&
			// 	t.DestIP == ip.DstIP.String() &&
			// 	t.DestPort == uint32(tcp.DstPort) {
			// 	found = true
			// 	fmt.Printf("%s %d %s %d == %s %d %s %d\n", ip.SrcIP.String(), tcp.SrcPort, ip.DstIP.String(), tcp.DstPort, t.SourceIP, t.SourcePort, t.DestIP, t.DestPort)

			// }
			if (ip.SrcIP.String() == t.SourceIP && tcp.SrcPort == layers.TCPPort(t.SourcePort)) &&
				(ip.DstIP.String() == t.DestIP && tcp.DstPort == layers.TCPPort(t.DestPort)) {
				fmt.Printf("ingress: %s %d %s %d == %s %d %s %d\n", ip.SrcIP.String(), tcp.SrcPort, ip.DstIP.String(), tcp.DstPort, t.SourceIP, t.SourcePort, t.DestIP, t.DestPort)
				ingress = true
			}
			if (ip.DstIP.String() == t.SourceIP && tcp.DstPort == layers.TCPPort(t.SourcePort)) &&
				(ip.SrcIP.String() == t.DestIP && tcp.SrcPort == layers.TCPPort(t.DestPort)) {
				fmt.Printf("egress: %s %d %s %d == %s %d %s %d\n", ip.SrcIP.String(), tcp.SrcPort, ip.DstIP.String(), tcp.DstPort, t.SourceIP, t.SourcePort, t.DestIP, t.DestPort)
				egress = true
			}
			// If it matches then reset it (match in the other direction too)

			// if t.SourceIP == ip.DstIP.String() &&
			// 	t.SourcePort == uint32(tcp.DstPort) &&
			// 	t.DestIP == ip.SrcIP.String() &&
			// 	t.DestPort == uint32(tcp.SrcPort) {
			// 	egress = true
			// 	fmt.Printf("2: %s %d %s %d / %s %d %s %d\n", t.SourceIP, t.SourcePort, t.DestIP, t.DestPort, ip.SrcIP.String(), tcp.SrcPort, ip.DstIP.String(), tcp.DstPort)

			// }
			if ingress || egress {
				for i := 0; i < count; i++ {
					seq := tcp.Ack + uint32(i)*uint32(tcp.Window)
					err := sendRST(eth.DstMAC, eth.SrcMAC, ip.DstIP, ip.SrcIP, tcp.DstPort, tcp.SrcPort, seq, handle)
					if err != nil {
						return err
					}
				}
				if egress && ingress { // when both sides of communication is reset return
					return nil
				}
			}
		}
	}

	return nil
}
