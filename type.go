// Taken from github.com/coredns/plugin/forward/type.go with modification

package dnsredir

import "net"

type transportType int

const (
	typeUdp transportType = iota
	typeTcp
	typeTls
	typeTotalCount	// Dummy type
)

func stringToTransportType(s string) transportType {
	switch s {
	case "udp":
		return typeUdp
	case "tcp":
		return typeTcp
	case "tcp-tls":
		return typeTls
	}

	log.Warningf("Unknown protocol %q, fallback to UDP", s)
	return typeUdp
}

func (t *Transport) transportTypeFromConn(pc *persistConn) transportType {
	if _, ok := pc.c.Conn.(*net.UDPConn); ok {
		return typeUdp
	}

	if t.tlsConfig == nil {
		return typeTcp
	}

	return typeTls
}

