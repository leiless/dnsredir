package dnsredir

import (
	"context"
	"fmt"
	"github.com/coredns/coredns/plugin"
	"hash/fnv"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"
)

const pluginName = "dnsredir"

var (
	pluginVersion = "?"
	pluginHeadCommit = "?"
)

func PluginError(err error) error {
	return plugin.Error(pluginName, err)
}

func MinUint32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
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

// Return two strings delimited by the `c', the second one will including `c' as beginning character
// If `c' not found in `s', the second string will be empty
func SplitByByte(s string, c byte) (string, string) {
	i := strings.IndexByte(s, c)
	if i >= 0 {
		return s[:i], s[i:]
	}
	return s, ""
}

// bootstrap: Bootstrap DNS to resolve domain names(empty array to use system defaults)
//
// see:
//	https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
//	https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
func getUrlContent(url, contentType string, bootstrap []string, timeout time.Duration) (string, error) {
	var transport http.RoundTripper

	if len(bootstrap) != 0 {
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				var d net.Dialer
				// Randomly choose a bootstrap DNS to resolve upstream host(if any)
				addr := bootstrap[rand.Intn(len(bootstrap))]
				return d.DialContext(ctx, network, addr)
			},
		}
		dialer := &net.Dialer{
			Timeout: timeout,
			Resolver: resolver,
		}
		// see: http.DefaultTransport
		transport = &http.Transport{
			DialContext:           dialer.DialContext,
			ExpectContinueTimeout: 1 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			Proxy:                 http.ProxyFromEnvironment,
			TLSHandshakeTimeout:   timeout,
		}
	} else {
		// Fallback to use system default resolvers, which located at /etc/resolv.conf
	}

	c := &http.Client{
		Transport: transport,	// [sic] If nil, DefaultTransport is used.
		Timeout:   timeout,		// Q: Should be omit this field if transport isn't nil?
	}
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

