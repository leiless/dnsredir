package dnsredir

import (
	"github.com/miekg/dns"
	"strings"
	"testing"
	"time"
)

const (
	defaultProto = ""
	udpProto = "udp"
	tcpProto = "tcp"
	tcpTlsProto = "tcp-tls"
	ms = time.Millisecond
	s = time.Second
)

func TestSend(t *testing.T) {
	tests := []struct {
		addr string
		proto string
		timeout time.Duration
		shouldErr bool
		expectedErr string	// Should be lower cased
	}{
		// Positive
		{"223.5.5.5:53", defaultProto, 500 * ms, false, ""},
		{"223.5.5.5:53", tcpProto, 500 * ms, false, ""},
		{"223.6.6.6:53", udpProto, 500 * ms, false, ""},
		{"223.6.6.6:53", tcpProto, 500 * ms, false, ""},
		{"127.0.0.1:53", udpProto, 30 * ms, false, ""},
		{"127.0.0.1:53", tcpProto, 50 * ms, false, ""},
		{"101.6.6.6:5353", tcpProto, 500 * ms, false, ""},
		{"119.29.29.29:53", udpProto, 500 * ms, false, ""},
		{"8.8.8.8:53", udpProto, 1 * s, false, ""},
		{"8.8.4.4:53", tcpProto, 1 * s, false, ""},
		{"8.8.8.8:853", tcpTlsProto, 1 * s, false, ""},
		{"8.8.4.4:853", tcpTlsProto, 1 * s, false, ""},
		{"1.0.0.1:53", udpProto, 1 * s, false, ""},
		{"1.1.1.1:53", defaultProto, 1 * s, false, ""},
		{"1.1.1.1:53", tcpProto, 1 * s, false, ""},
		{"1.0.0.1:853", tcpTlsProto, 1 * s, false, ""},
		{"1.1.1.1:853", tcpTlsProto, 1 * s, false, ""},
		{"9.9.9.9:853", tcpTlsProto, 500 * ms, false, ""},
		{"223.5.5.5:853", tcpTlsProto, 500 * ms, false, ""},
		{"223.6.6.6:853", tcpTlsProto, 500 * ms, false, ""},
		// Negative
		{"223.5.5.5", defaultProto, 500 * ms, true, "missing port in address"},
		{"127.0.0.1:853", tcpTlsProto, 100 * ms, true, "connection refused"},
		// DNSPod doesn't support DNS over TCP/TLS
		{"119.29.29.29:53", tcpProto, 500 * ms, true, "connection refused"},
		{"119.29.29.29:853", tcpTlsProto, 1 * s, true, "i/o timeout"},
		{"114.114.114.114", "foobar", 1 * s, true, "unknown network "},
	}

	for i, test := range tests {
		uh := &UpstreamHost{
			addr: test.addr,
			c: &dns.Client{
				Net: test.proto,
				Timeout: test.timeout,
			},
		}
		err := uh.Check()

		if test.shouldErr == (err != nil) {
			if err != nil {
				if !strings.Contains(strings.ToLower(err.Error()), test.expectedErr) {
					t.Errorf("Test#%v> expectErr: %v got: %v", i, test.expectedErr, err)
				} else {
					//t.Logf("Test#%v> error: %v", i, err)
				}
			}
		} else {
			t.Errorf("Test#%v> shouldErr: %v error: %v", i, test.shouldErr, err)
		}
	}
}

