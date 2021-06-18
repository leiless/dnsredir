package dnsredir

import (
	"testing"
)

func TestStringToDomain(t *testing.T) {
	tests := []struct {
		input          string
		shouldOk       bool
		expectedOutput string
	}{
		{"", false, ""},
		{".", false, ""},
		{"..", false, ""},
		{"-", false, ""},
		{"@", false, ""},
		{"+", false, ""},
		{"_", false, ""},
		{"a", true, "a"},
		{"A", true, "a"},
		{"cn", true, "cn"},
		{"IO", true, "io"},
		{"Io.", true, "io"},
		{"oRg.", true, "org"},
		{"oRg.", true, "org"},
		{"wikipedia.org.", true, "wikipedia.org"},
		{"google.com", true, "google.com"},
		{"TWITTER.COM.", true, "twitter.com"},
		{"TWITTER.COM..", false, ""},
		{"G00GLE.", true, "g00gle"},
		{"a..ma.zon", false, ""},
		{"a.ma.zon", true, "a.ma.zon"},
		{"A.ma.ZON.", true, "a.ma.zon"},
		{".A.ma.ZON.", false, ""},
		{"..A.ma.ZON.", false, ""},
		{"...a.ma.z0n.", false, ""},
		{"foo.-bar", false, ""},
		{"foo-.bar", false, ""},
		{"foo-bar.", true, "foo-bar"},
		{".foo-bar.", false, ""},
		{"foo.XN--abcde0xdead", true, "foo.xn--abcde0xdead"},
		{"foo.XN--abcde0xdead.", true, "foo.xn--abcde0xdead"},
		{"foo.XN-.abcde0xdead.", false, ""},
		{"0", true, "0"},
		{"0.123", true, "0.123"},
		{"0-123", true, "0-123"},
		{"0-0", true, "0-0"},
		{"0-", false, ""},
		{"-a", false, ""},
		// Maximum characters per section: 63
		{"SDsadjkDSAsdaSDJASdasd1311839123-021CD123u1900-21j3i231oi1sW-dt.cache.org.", true, "sdsadjkdsasdasdjasdasd1311839123-021cd123u1900-21j3i231oi1sw-dt.cache.org"},
		// 64 characters
		{"SDsadjkDSAsdaSDJASdasd1311839123-021CD123u1900-21j3i231oi1sW-dt9", false, ""},
	}
	for i, c := range tests {
		if domain, ok := stringToDomain(c.input); ok != c.shouldOk || domain != c.expectedOutput {
			t.Errorf("Test case#%v failed, %v %q vs %v %q", i, ok, domain, c.shouldOk, c.expectedOutput)
		}
	}
}
