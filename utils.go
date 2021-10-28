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
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const pluginName = "dnsredir"

var (
	pluginVersion    = "?"
	pluginHeadCommit = "?"
)

var userAgent = fmt.Sprintf("coredns-%v %v %v", pluginName, pluginVersion, pluginHeadCommit)

const (
	mimeTypeDohAny           = "?/?" // Dummy MIME type
	mimeTypeJson             = "application/json"
	mimeTypeDnsJson          = "application/dns-json"
	mimeTypeDnsMessage       = "application/dns-message"
	mimeTypeDnsUdpWireFormat = "application/dns-udpwireformat"
	headerAccept             = mimeTypeDnsMessage + ", " + mimeTypeDnsJson + ", " + mimeTypeDnsUdpWireFormat + ", " + mimeTypeJson
)

type Once int32

func (o *Once) Do(f func()) {
	if o != nil && atomic.CompareAndSwapInt32((*int32)(o), 0, 1) {
		f()
	}
}

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

		if seg[0] == '-' || seg[n-1] == '-' {
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

func isContentType(contentType string, h *http.Header) bool {
	t := h.Get("Content-Type")
	return t == contentType || strings.Contains(t, contentType+";")
}

// bootstrap: Bootstrap DNS to resolve domain names(empty array to use system defaults)
//
// see:
//	https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
//	https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
func getUrlContent(theUrl, contentType string, bootstrap []string, timeout time.Duration) (string, error) {
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
			Timeout:  timeout,
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

	req, err := http.NewRequest(http.MethodGet, theUrl, nil)
	if err != nil {
		return "", err
	}
	// Set a fake user agent in case of access denied error
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:80.0) Gecko/20100101 Firefox/80.0")

	c := &http.Client{
		Transport: transport, // [sic] If nil, DefaultTransport is used.
		Timeout:   timeout,   // Q: Should we omit this field if transport isn't nil?
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer Close(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status code: %v", resp.StatusCode)
	}

	if len(contentType) != 0 && !isContentType(contentType, &resp.Header) {
		if theUrl, err = fixUrl(theUrl, resp.Header); err != nil {
			return "", err
		} else {
			return getUrlContent(theUrl, contentType, bootstrap, timeout)
		}
	}

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	// We don't use http.DetectContentType()
	return string(content), nil
}

func fixUrl(theUrl string, h http.Header) (string, error) {
	const LocationKey = "Location"
	location := h.Get(LocationKey)
	if location != "" {
		if _, err := url.Parse(theUrl); err != nil {
			return "", fmt.Errorf("fixUrl(): url.Parse(): %w", err)
		}
		return location, nil
	}
	return "", fmt.Errorf("%q header key not found in %v", LocationKey, theUrl)
}

func stringHash(str string) uint64 {
	h := fnv.New64a()
	_, err := h.Write([]byte(str))
	if err != nil {
		panic(err)
	}
	return h.Sum64()
}

func hostPortIsIpPort(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	// Strip the zone, see: coredns/plugin/pkg/parse/host.go#stripZone()
	i := strings.IndexByte(host, '%')
	return i > 0 && net.ParseIP(host[:i]) != nil
}
