package dnsredir

import (
	"fmt"
	"github.com/miekg/dns"
	"strings"
	"testing"
	"time"
)

const (
	defaultProto = "" // Alias to UDP
	udpProto     = "udp"
	tcpProto     = "tcp"
	tcpTlsProto  = "tcp-tls"

	ms = time.Millisecond
	s  = time.Second
)

type testCaseSend struct {
	addr        string
	proto       string
	timeout     time.Duration
	shouldErr   bool
	expectedErr string
}

func (t testCaseSend) String() string {
	return fmt.Sprintf("{%T addr=%v proto=%v timeout=%v shouldErr=%v expectedErr=%q}",
		t, t.addr, t.proto, t.timeout, t.shouldErr, t.expectedErr)
}

// Return true if test passed, false otherwise
func (t *testCaseSend) Pass(err error) bool {
	// Empty expected error isn't allow
	if t.shouldErr == (len(t.expectedErr) == 0) {
		panic(fmt.Sprintf("Bad test case %v", t))
	}

	pass := true
	if t.shouldErr == (err != nil) {
		if err != nil {
			s := strings.ToLower(err.Error())
			t := strings.ToLower(t.expectedErr)
			if !strings.Contains(s, t) {
				pass = false
			}
		}
	} else {
		pass = false
	}
	return pass
}

func TestSend(t *testing.T) {
	tests := []testCaseSend{
		// Positive
		{"8.8.8.8:53", udpProto, 1 * s, false, ""},
		{"8.8.4.4:53", tcpProto, 1 * s, false, ""},
		{"8.8.8.8:853", tcpTlsProto, 1 * s, false, ""},
		{"1.1.1.1:53", defaultProto, 1 * s, false, ""},
		{"9.9.9.9:53", tcpProto, 1 * s, false, ""},
		// Negative
		{"1.2.3.4", defaultProto, 500 * ms, true, "missing port in address"},
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
				Net:     test.proto,
				Timeout: test.timeout,
			},
			transport: newTransport(),
		}
		err := uh.Check()
		if !test.Pass(err) {
			t.Errorf("Test#%v failed  %v vs err: %v", i, test, err)
		}
	}
}
