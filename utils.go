package dnsredir

import (
	"fmt"
	"github.com/coredns/coredns/plugin"
	"hash/fnv"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

const (
	pluginName = "dnsredir"
	pluginVersion = "0.0.1"
)

var pluginHeadCommit = "?"

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

// Return two strings delimited by the `c', which the `c' won't be included
// If `c' not found in `s', `s' and an empty string will be returned
func SplitByByte(s string, c byte) (string, string) {
	i := strings.IndexByte(s, c)
	if i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// see:
//	https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
//	https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
func getUrlContent(url, contentType string, timeout time.Duration) (string, error) {
	c := &http.Client{ Timeout: timeout }
	resp, err := c.Get(url)
	if err != nil {
		return "", err
	}
	defer Close(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status code: %v", resp.StatusCode)
	}

	contentType1 := resp.Header.Get("Content-Type")
	if len(contentType) != 0 {
		if contentType1 != contentType && !strings.Contains(contentType1, contentType + ";") {
			return "", fmt.Errorf("bad Content-Type, expect: %q got: %q", contentType, contentType1)
		}
	}

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	// We don't use http.DetectContentType()
	return string(content), nil
}

func stringHash(str string) uint64 {
	h := fnv.New64a()
	_, err := h.Write([]byte(str))
	if err != nil {
		panic(err)
	}
	return h.Sum64()
}

