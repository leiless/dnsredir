package redirect

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
)

func TestSend(t *testing.T) {
	tests := []struct {
		host string
		proto string
		timeout time.Duration
		shouldErr bool
		expectedErr string	// Should be lower cased
	}{
		// Positive
		{"223.5.5.5:53", defaultProto, 500 * time.Millisecond, false, ""},
		{"223.5.5.5:53", tcpProto, 500 * time.Millisecond, false, ""},
		{"223.6.6.6:53", udpProto, 500 * time.Millisecond, false, ""},
		{"223.6.6.6:53", tcpProto, 500 * time.Millisecond, false, ""},
		{"127.0.0.1:53", udpProto, 30 * time.Millisecond, false, ""},
		{"127.0.0.1:53", tcpProto, 50 * time.Millisecond, false, ""},
		{"101.6.6.6:5353", tcpProto, 500 * time.Millisecond, false, ""},
		{"119.29.29.29:53", udpProto, 500 * time.Millisecond, false, ""},
		// Negative
		{"223.5.5.5", defaultProto, 500 * time.Millisecond, true, "missing port in address"},
		{"127.0.0.1:853", tcpTlsProto, 100 * time.Millisecond, true, "connection refused"},
		// DNSPod doesn't support DNS over TCP
		{"119.29.29.29:53", tcpProto, 500 * time.Millisecond, true, "connection refused"},
	}

	for i, test := range tests {
		uh := &UpstreamHost{
			host: test.host,
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

