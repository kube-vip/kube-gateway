package manager

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
	"time"

	"github.com/gookit/slog"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	"github.com/vishvananda/netlink/nl"

	//"github.com/gopacket/gopacket/pcap"
	//gopcap "github.com/packetcap/go-pcap"
	"github.com/evilsocket/opensnitch/daemon/netlink"
)

type Tuple struct {
	SourceIP   string
	SourcePort uint32
	DestIP     string
	DestPort   uint32
}

func sendRST(srcMac, dstMac net.HardwareAddr, srcIP, dstIP net.IP, srcPort, dstPort layers.TCPPort, seq uint32, handle *pcap.Handle) error {
	// log.Printf("send %v:%v > %v:%v [RST] seq %v", srcIP.String(), srcPort.String(), dstIP.String(), dstPort.String(), seq)

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
			slog.Info("Flushing connections (timed out)")
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

			if (ip.SrcIP.String() == t.SourceIP && tcp.SrcPort == layers.TCPPort(t.SourcePort)) &&
				(ip.DstIP.String() == t.DestIP && tcp.DstPort == layers.TCPPort(t.DestPort)) {
				fmt.Printf("flushing ingress: %s:%d %s:%d == %s:%d %s:%d\n", ip.SrcIP.String(), tcp.SrcPort, ip.DstIP.String(), tcp.DstPort, t.SourceIP, t.SourcePort, t.DestIP, t.DestPort)
				ingress = true
			}
			if (ip.DstIP.String() == t.SourceIP && tcp.DstPort == layers.TCPPort(t.SourcePort)) &&
				(ip.SrcIP.String() == t.DestIP && tcp.SrcPort == layers.TCPPort(t.DestPort)) {
				fmt.Printf("flushing egress: %s:%d %s:%d == %s:%d %s:%d\n", ip.SrcIP.String(), tcp.SrcPort, ip.DstIP.String(), tcp.DstPort, t.SourceIP, t.SourcePort, t.DestIP, t.DestPort)
				egress = true
			}

			if ingress || egress {
				for i := 0; i < count; i++ {
					seq := tcp.Ack + uint32(i)*uint32(tcp.Window)
					err := sendRST(eth.DstMAC, eth.SrcMAC, ip.DstIP, ip.SrcIP, tcp.DstPort, tcp.SrcPort, seq, handle)
					if err != nil {
						return err
					}
				}
				if egress && ingress { // when both sides of communication is reset return
					socks, err := netlink.SocketsDump(syscall.AF_INET, syscall.IPPROTO_TCP)
					if err != nil {
						slog.Errorf("Dumping sockets %v", err)
					} else {
						for x := range socks {

							if !socks[x].ID.Source.IsLoopback() {
								sockReq := &SocketRequest{
									Family:   socks[x].Family,
									Protocol: syscall.IPPROTO_TCP,
									ID:       socks[x].ID,
								}

								req := nl.NewNetlinkRequest(nl.SOCK_DESTROY, syscall.NLM_F_REQUEST|syscall.NLM_F_ACK)
								req.AddData(sockReq)
								_, err := req.Execute(syscall.NETLINK_INET_DIAG, 0)
								if err != nil {
									slog.Errorf("Destroy sockets %v", err)
								}
							}
						}
					}

					netlink.KillSocket("tcp", ip.SrcIP, uint(tcp.SrcPort.LayerType()), ip.DstIP, uint(tcp.DstPort)) // Double down on the death to sockets

					return nil

				}
			}
		}
	}

	return nil
}

// SocketID holds the socket information of a request/response to the kernel
type SocketID struct {
	Source          net.IP
	Destination     net.IP
	Cookie          [2]uint32
	Interface       uint32
	SourcePort      uint16
	DestinationPort uint16
}

// Socket represents a netlink socket.
type Socket struct {
	ID      SocketID
	Expires uint32
	RQueue  uint32
	WQueue  uint32
	UID     uint32
	INode   uint32
	Family  uint8
	State   uint8
	Timer   uint8
	Retrans uint8
}

// SocketRequest holds the request/response of a connection to the kernel
type SocketRequest struct {
	ID       netlink.SocketID
	States   uint32
	Family   uint8
	Protocol uint8
	Ext      uint8
	pad      uint8
}

type writeBuffer struct {
	Bytes []byte
	pos   int
}

func (b *writeBuffer) Write(c byte) {
	b.Bytes[b.pos] = c
	b.pos++
}

func (b *writeBuffer) Next(n int) []byte {
	s := b.Bytes[b.pos : b.pos+n]
	b.pos += n
	return s
}

const sizeofSocketRequest = sizeofSocketID + 0x8
const sizeofSocketID = 0x30
const sizeofSocket = sizeofSocketID + 0x18

// Serialize convert SocketRequest struct to bytes.
func (r *SocketRequest) Serialize() []byte {
	b := writeBuffer{Bytes: make([]byte, sizeofSocketRequest)}
	b.Write(r.Family)
	b.Write(r.Protocol)
	b.Write(r.Ext)
	b.Write(r.pad)
	nl.NativeEndian().PutUint32(b.Next(4), r.States)
	binary.BigEndian.PutUint16(b.Next(2), r.ID.SourcePort)
	binary.BigEndian.PutUint16(b.Next(2), r.ID.DestinationPort)
	if r.Family == syscall.AF_INET6 {
		copy(b.Next(16), r.ID.Source)
		copy(b.Next(16), r.ID.Destination)
	} else {
		copy(b.Next(16), r.ID.Source.To4())
		copy(b.Next(16), r.ID.Destination.To4())
	}
	nl.NativeEndian().PutUint32(b.Next(4), r.ID.Interface)
	nl.NativeEndian().PutUint32(b.Next(4), r.ID.Cookie[0])
	nl.NativeEndian().PutUint32(b.Next(4), r.ID.Cookie[1])
	return b.Bytes
}

// Len returns the size of a socket request
func (r *SocketRequest) Len() int { return sizeofSocketRequest }

type readBuffer struct {
	Bytes []byte
	pos   int
}

func (b *readBuffer) Read() byte {
	c := b.Bytes[b.pos]
	b.pos++
	return c
}

func (b *readBuffer) Next(n int) []byte {
	s := b.Bytes[b.pos : b.pos+n]
	b.pos += n
	return s
}

func (s *Socket) deserialize(b []byte) error {
	if len(b) < sizeofSocket {
		return fmt.Errorf("socket data short read (%d); want %d", len(b), sizeofSocket)
	}
	rb := readBuffer{Bytes: b}
	s.Family = rb.Read()
	s.State = rb.Read()
	s.Timer = rb.Read()
	s.Retrans = rb.Read()
	s.ID.SourcePort = binary.BigEndian.Uint16(rb.Next(2))
	s.ID.DestinationPort = binary.BigEndian.Uint16(rb.Next(2))
	if s.Family == syscall.AF_INET6 {
		s.ID.Source = net.IP(rb.Next(16))
		s.ID.Destination = net.IP(rb.Next(16))
	} else {
		s.ID.Source = net.IPv4(rb.Read(), rb.Read(), rb.Read(), rb.Read())
		rb.Next(12)
		s.ID.Destination = net.IPv4(rb.Read(), rb.Read(), rb.Read(), rb.Read())
		rb.Next(12)
	}
	s.ID.Interface = nl.NativeEndian().Uint32(rb.Next(4))
	s.ID.Cookie[0] = nl.NativeEndian().Uint32(rb.Next(4))
	s.ID.Cookie[1] = nl.NativeEndian().Uint32(rb.Next(4))
	s.Expires = nl.NativeEndian().Uint32(rb.Next(4))
	s.RQueue = nl.NativeEndian().Uint32(rb.Next(4))
	s.WQueue = nl.NativeEndian().Uint32(rb.Next(4))
	s.UID = nl.NativeEndian().Uint32(rb.Next(4))
	s.INode = nl.NativeEndian().Uint32(rb.Next(4))
	return nil
}
