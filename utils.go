package redirect

import (
	"github.com/coredns/coredns/plugin"
	"io"
	"strings"
)

const pluginName = "redirect"

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

func RemoveTrailingDot(s string) string {
	if n := len(s); n > 0 && s[n-1] == '.' {
		return s[:n-1]
	}
	return s
}

/**
 * Rough check if `s' is a domain name
 * XXX: it won't honor valid TLD and Punycode
 */
func IsDomainName(s string) bool {
	s = RemoveTrailingDot(strings.ToLower(s))
	f := strings.FieldsFunc(s, func(r rune) bool {
		return r == '.'
	})

	if len(f) < 2 {
		return false
	}

	for _, seg := range f {
		n := len(seg)
		if n == 0 || n > 63 {
			return false
		}

		for _, c := range seg {
			if c != '-' && (c < '0' || c > '9') && (c < 'a' || c > 'z') {
				return false
			}
		}

		if seg[0] == '-' || seg[n - 1] == '-' {
			return false
		}
	}

	return true
}

