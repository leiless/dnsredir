package dnsredir

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/coredns/coredns/plugin/pkg/transport"
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

var knownTrans = []string{
	"dns", // Use protocol specified in incoming DNS requests, i.e. it may UDP, TCP.
	"udp",
	"tcp",
	"tls",
	"json-doh",
	"ietf-doh",
	"ietf-http-doh",
	"doh",
}

func SplitTransportHost(s string) (trans string, addr string) {
	s = strings.ToLower(s)
	for _, trans := range knownTrans {
		if strings.HasPrefix(s, trans+"://") {
			return trans, s[len(trans+"://"):]
		}
	}
	// Have no proceeding transport? assume it's classic DNS protocol
	return "dns", s
}

// Taken from parse.HostPortOrFile() with modification
func HostPort(servers []string) ([]string, error) {
	var list []string
	for _, h := range servers {
		trans, host := SplitTransportHost(h)
		addr, _, err := net.SplitHostPort(host)
		if err != nil {
			if strings.HasSuffix(trans, "doh") {
				if _, err := url.ParseRequestURI(h); err != nil {
					return nil, fmt.Errorf("failed to parse %q: %v", h, err)
				}
			} else {
				// Parse didn't work, it is not an addr:port combo
				if _, ok := stringToDomain(host); !ok && net.ParseIP(stripZoneAndTlsName(host)) == nil {
					return nil, fmt.Errorf("#1 not a domain name or an IP address: %q", host)
				}
			}

			var s string
			switch trans {
			case "dns":
				fallthrough
			case "udp":
				fallthrough
			case "tcp":
				s = trans + "://" + net.JoinHostPort(host, transport.Port)
			case "tls":
				host, tlsName := SplitByByte(host, '@')
				s = trans + "://" + net.JoinHostPort(host, transport.TLSPort) + tlsName
			case "json-doh":
				fallthrough
			case "ietf-doh":
				fallthrough
			case "ietf-http-doh":
				fallthrough
			case "doh":
				s = h
			default:
				panic(fmt.Sprintf("Unknown transport %q", trans))
			}
			list = append(list, s)
		} else {
			if _, ok := stringToDomain(addr); !ok && net.ParseIP(stripZoneAndTlsName(addr)) == nil {
				return nil, fmt.Errorf("#2 not a domain name or an IP address: %q", host)
			}
			list = append(list, trans+"://"+host)
		}
	}
	return list, nil
}
