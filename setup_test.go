package dnsredir

import (
	"fmt"
	"github.com/coredns/caddy"
	"strings"
	"testing"
)

type testCase struct {
	input       string
	shouldErr   bool
	expectedErr string
}

func (t testCase) String() string {
	return fmt.Sprintf("{%T input=%q shouldErr=%v expectedErr=%q}",
		t, t.input, t.shouldErr, t.expectedErr)
}

// Return true if test passed, false otherwise
func (t *testCase) Pass(err error) bool {
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

func TestSetupTo(t *testing.T) {
	tests := []testCase{
		// Negative
		{"dnsredir", true, `missing mandatory property: "to"`},
		{"dnsredir .", true, `missing mandatory property: "to"`},
		{"dnsredir whitelist.zone {}", true, `missing mandatory property: "to"`},
		{"dnsredir root.zone { }", true, `missing mandatory property: "to"`},
		{"dnsredir . { to }", true, "not an IP address:"},
		{"dnsredir . { to . }", true, "not an IP address:"},
		{"dnsredir . { to foo }", true, "not an IP address:"},
		{"dnsredir . { to foobar.net }", true, "not an IP address:"},
		{"dnsredir . { to foobar.net. }", true, "not an IP address:"},
		{"dnsredir . { to 1.2.3 }", true, "not an IP address:"},
		{"dnsredir . { to 1.2.3.4 }", true, "not an IP address:"},
		{"dnsredir . { to \n }", true, "Wrong argument count or unexpected line ending after"},
		{"dnsredir . { to . \n }", true, "not an IP address:"},
		{"dnsredir . { to / \n }", true, "not an IP address:"},
		{"dnsredir . { to 1.2.3. \n }", true, "not an IP address:"},
		{"dnsredir . { to foobar://1.1.1.1 \n }", true, "not an IP address:"},
		// Positive
		{"dnsredir . { to 1.2.3.4 \n }", false, ""},
		{"dnsredir . { to 1.2.3.4 / . \n }", true, "not an IP address:"},
		{"dnsredir . { to dns://8.8.4.4 \n }", false, ""},
		{"dnsredir . { to dns://192.168.144.10:5353 \n }", false, ""},
		{"dnsredir . { to tls://172.16.10.1 \n }", false, ""},
		{"dnsredir . { to tls://172.16.10.1:1234 \n }", false, ""},
		{"dnsredir . { to 10.1.2.3 dns://192.168.144.100 tls://172.16.10.1:1234 \n }", false, ""},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		_, err := newReloadableUpstream(c)
		if !test.Pass(err) {
			t.Errorf("Test#%v failed  %v vs err: %v", i, test, err)
		}
	}
}
