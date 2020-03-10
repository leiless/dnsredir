package dnsredir

import (
	"github.com/coredns/coredns/plugin"
	"io"
	"strings"
)

const pluginName = "dnsredir"

// Used for generic expressions and statements
func Unused(arg0 interface{}, args ...interface{}) {
	// Just consume all arguments
	if arg0 == nil {
		for i := range args {
			if i == 0 {
				break
			}
		}
	}
}

var (
	UnusedParam = Unused
	UnusedResult = Unused
)

func PluginError(err error) error {
	return plugin.Error(pluginName, err)
}

// see: https://blevesearch.com/news/Deferred-Cleanup,-Checking-Errors,-and-Potential-Problems/
func Close(c io.Closer) {
	err := c.Close()
	if err != nil {
		log.Warningf("%v", err)
	}
}

/**
 * Rough check if `s' is a domain name
 * XXX: it won't honor valid TLD and Punycode
 */
func isDomainName(s string) bool {
	f := strings.Split(s, ".")

	// len(f) == 1 means a TLD
	if len(f) == 0 {
		return false
	}

	for _, seg := range f {
		n := len(seg)
		if n == 0 || n > 63 {
			return false
		}

		for _, c := range seg {
			// More specifically, TLD should only contain [a-z] and hyphen
			// We currently don't have such constrain
			if c != '-' && c != '_' && (c < '0' || c > '9') && (c < 'a' || c > 'z') {
				return false
			}
		}

		if seg[0] == '-' || seg[n - 1] == '-' {
			return false
		}
	}

	return true
}

func removeTrailingDot(s string) string {
	if n := len(s); n > 0 && s[n-1] == '.' {
		return s[:n-1]
	}
	return s
}

// Try to convert a string to a domain name
// Returned string is lower cased and without trailing dot
// Empty string is returned if it's not a domain name
func stringToDomain(s string) (string, bool) {
	s = removeTrailingDot(strings.ToLower(s))
	if isDomainName(s) {
		return s, true
	}
	return "", false
}

// Return two strings delimited by the `c'
// If `c' not found in `s', `s' and an empty string will be returned
func SplitByByte(s string, c byte) (string, string) {
	i := strings.IndexByte(s, c)
	if i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

