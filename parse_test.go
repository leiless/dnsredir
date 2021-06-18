package dnsredir

import (
	"strings"
	"testing"
)

func TestHostPort(T *testing.T) {
	var servers = []string{
		"127.0.0.1",
		"127.0.0.1:5353",
		"2001:db8:85a3::8a2e:370:7334",
		"[2001:db8:85a3::8a2e:370:7334]:5353",
		"2001:db8:85a3::8a2e:370:7334%foobar",
		"[2001:db8:85a3::8a2e:370:7334%foobar]:5353",
		"dns://192.168.1.2",
		"dns://192.168.1.2:1053",
		"dns://fe80::1ff:fe23:4567:890a",
		"dns://[fe80::1ff:fe23:4567:890a]:1053",
		"dns://fe80::1ff:fe23:4567:890a%lo0",
		"dns://[fe80::1ff:fe23:4567:890a%lo0]:1053",
		"udp://172.16.10.1",
		"udp://172.16.10.1:530",
		"udp://2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		"udp://[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:530",
		"udp://2001:0db8:85a3:0000:0000:8a2e:0370:7334%lo1",
		"udp://[2001:0db8:85a3:0000:0000:8a2e:0370:7334%lo1]:530",
		"tcp://10.1.2.3",
		"tcp://10.1.2.3:1234",
		"tcp://::1",
		"tcp://[::1]:1234",
		"tls://1.2.3.4",
		"tls://1.2.3.4:5353",
		"tls://::1",
		"tls://::1@foobar.net",
		"tls://[::1]:1234@foobar.net",
		"tls://::1%eth0",
		"tls://::1%eth0@foobar.net",
		"tls://[::1%eth0]:1234",
		"tls://[::1%eth0]:1234@foobar.net",
		"https://1.1.1.1",
		"https://1.1.1.1:5353",
		"https://::1",
		"https://[::1]:5353",
		"https://::1%eth1",
		"https://[::1%eth1]:5353",
	}
	hosts, err := HostPort(servers)
	if err != nil {
		T.Errorf("HostPort() fail, error: %v", err)
		return
	}
	if len(hosts) != len(servers) {
		T.Errorf("HostPort() fail, expected size %v, got %v, hosts: %v", len(servers), len(hosts), hosts)
	}

	for i, host := range hosts {
		s := servers[i]
		if strings.Contains(s, "://") {
			s = s[strings.Index(s, "://")+len("://"):]
		}

		// If host contain a TLS server name
		if strings.Contains(s, "@") {
			s, t := SplitByByte(s, '@')
			if !strings.Contains(host, s) {
				T.Errorf("Resolved %q doesn't contain %q", host, s)
				break
			}
			if !strings.Contains(host, t) {
				T.Errorf("Resolved %q doesn't contain %q", host, t)
				break
			}
		} else {
			if !strings.Contains(host, s) {
				T.Errorf("Resolved %q doesn't contain %q", host, s)
				break
			}
		}
	}
}
