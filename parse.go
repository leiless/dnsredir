package dnsredir

import (
	"fmt"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"net"
	"strings"
)

// Strips the IPv6 zone and TLS server name
// If above two both present, the latter one should always comes after the former one.
// Return host address and TLS server name(if any)
func stripZoneAndTlsName(host string) (string, string) {
	if strings.Contains(host, "%") {
		i := strings.Index(host, "%")
		s := host[:i]
		t := host[i + 1:]
		_, t = SplitByByte(t, '@')
		return s, t
	}
	return SplitByByte(host, '@')
}

var knownTrans = []string {
	"dns",	// Alias of UDP protocol
	"udp",
	"tcp",
	"tls",
	"https",
}

func splitTransportHost(s string) (trans string, addr string) {
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
		trans, host := splitTransportHost(h)
		addr, _, err := net.SplitHostPort(host)
		if err != nil {
			// Parse didn't work, it is not an addr:port combo
			host1, tlsName := stripZoneAndTlsName(host)
			if net.ParseIP(host1) == nil {
				return nil, fmt.Errorf("#1 not an IP address: %q", host)
			}

			var s string
			switch trans {
			case "udp":
				fallthrough
			case "tcp":
				s = trans + "://" + net.JoinHostPort(host, transport.Port)
			case "tls":
				if len(tlsName) != 0 {
					tlsName = "@" + tlsName
				}
				s = trans + "://" + net.JoinHostPort(host1, transport.TLSPort) + tlsName
			case "https":
				s = trans + "://" + net.JoinHostPort(host, transport.HTTPSPort)
			default:
				panic(fmt.Sprintf("Unknown transport %q", trans))
			}
			list = append(list, s)
			continue
		}

		addr1, _ := stripZoneAndTlsName(addr)
		if net.ParseIP(addr1) == nil {
			return nil, fmt.Errorf("#2 not an IP address: %q", host)
		}
		list = append(list, trans + "://" + host)
	}
	return list, nil
}

