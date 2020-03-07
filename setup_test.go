package dnsredir

import (
	"fmt"
	"github.com/caddyserver/caddy"
	"strings"
	"testing"
)

type testCase struct {
	input string
	shouldErr bool
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

func TestSetup(t *testing.T) {
	tests := []testCase {
		// Negative
		{"dnsredir", true, `missing mandatory property: "to"`},
		{"dnsredir .", true, `missing mandatory property: "to"`},
		{"dnsredir whitelist.zone {}", true, `missing mandatory property: "to"`},
		{"dnsredir root.zone { }", true, `missing mandatory property: "to"`},
		{"dnsredir . { to }", true, "not an IP address or file:"},
		{"dnsredir . { to . }", true, "not an IP address or file:"},
		{"dnsredir . { to foo }", true, "not an IP address or file:"},
		{"dnsredir . { to foobar.net }", true, "not an IP address or file:"},
		{"dnsredir . { to foobar.net. }", true, "not an IP address or file:"},
		{"dnsredir . { to 1.2.3 }", true, "not an IP address or file:"},
		{"dnsredir . { to 1.2.3.4 }", true, "not an IP address or file:"},
		{"dnsredir . { to \n }", true, "Wrong argument count or unexpected line ending after"},
		{"dnsredir . { to . \n }", true, "parsed from file(s), yet no valid entry was found"},
		{"dnsredir . { to / \n }", true, "parsed from file(s), yet no valid entry was found"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		r, err := newReloadableUpstream(c)
		Unused(r)
		if !test.Pass(err) {
			t.Errorf("Test#%v failed  %v vs err: %v", i, test, err)
		}
	}
}

