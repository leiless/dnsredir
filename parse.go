package dnsredir

import (
	"fmt"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"net"
	"strings"
)

// Strips trailing IP zone and/or TLS server name
// If above two both present, the former one should always comes before the latter one.
// see: https://www.ietf.org/rfc/rfc4001.txt
func stripZoneAndTlsName(host string) string {
	if strings.Contains(host, "%") {
		return host[:strings.Index(host, "%")]
	}
	if strings.Contains(host, "@") {
		return host[:strings.Index(host, "@")]
	}
	return host
}

var knownTrans = []string {
	"dns",	// Alias of UDP protocol
	"udp",
	"tcp",
	"tls",
}

func SplitTransportHost(s string) (trans string, addr string) {
	s = strings.ToLower(s)
	for _, trans := range knownTrans {
		if strings.HasPrefix(s, trans + "://") {
			if trans == "dns" {
				trans = "udp"
			}
			return trans, s[len(trans + "://"):]
		}
	}
	// Have no proceeding transport? assume it's classic DNS protocol
	return "udp", s
}

// Taken from parse.HostPortOrFile() with modification
func HostPort(servers []string) ([]string, error) {
	var list []string
	for _, h := range servers {
		trans, host := SplitTransportHost(h)
		addr, _, err := net.SplitHostPort(host)
		if err != nil {
			// Parse didn't work, it is not an addr:port combo
			if net.ParseIP(stripZoneAndTlsName(host)) == nil {
				return nil, fmt.Errorf("#1 not an IP address: %q", host)
			}

			var s string
			switch trans {
			case "udp":
				fallthrough
			case "tcp":
				s = trans + "://" + net.JoinHostPort(host, transport.Port)
			case "tls":
				host, tlsName := SplitByByte(host, '@')
				s = trans + "://" + net.JoinHostPort(host, transport.TLSPort) + tlsName
			default:
				panic(fmt.Sprintf("Unknown transport %q", trans))
			}
			list = append(list, s)
			continue
		}

		if net.ParseIP(stripZoneAndTlsName(addr)) == nil {
			return nil, fmt.Errorf("#2 not an IP address: %q", host)
		}
		list = append(list, trans + "://" + host)
	}
	return list, nil
}

