package dnsredir

import (
	"bytes"
	"context"
	"fmt"
	"github.com/coredns/coredns/plugin"
	"golang.org/x/net/html"
	"hash/fnv"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const pluginName = "dnsredir"

var (
	pluginVersion = "?"
	pluginHeadCommit = "?"
)

var userAgent = fmt.Sprintf("coredns-%v %v %v", pluginName, pluginVersion, pluginHeadCommit)

const (
	mimeTypeDohAny           = "?/?"	// Dummy MIME type
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

func isContentType(contentType string, h *http.Header) bool {
	t := h.Get("Content-Type")
	return t == contentType || strings.Contains(t, contentType + ";")
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

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	// Set a fake user agent in case of access denied error
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:80.0) Gecko/20100101 Firefox/80.0")

	c := &http.Client{
		Transport: transport,	// [sic] If nil, DefaultTransport is used.
		Timeout:   timeout,		// Q: Should be omit this field if transport isn't nil?
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
		u := strings.ToLower(url)
		// Dirty patch to fix t.cn not redirecting problem
		// see: https://github.com/leiless/dnsredir/issues/4
		if strings.HasPrefix(u, "https://t.cn/") && isContentType("text/html", &resp.Header) {
			if url, err := tcnFix(url, resp.Body); err != nil {
				return "", err
			} else {
				return getUrlContent(url, contentType, bootstrap, timeout)
			}
		} else {
			s := "Content-Type"
			return "", fmt.Errorf("bad %v, expect: %q got: %q", s, contentType, resp.Header.Get(s))
		}
	}

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	// We don't use http.DetectContentType()
	return string(content), nil
}

func tcnFix(url string, r io.Reader) (string, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return "", err
	}

	h, err := html.Parse(bytes.NewReader(b))

	var fixFunc func(*html.Node) (string, bool)
	fixFunc = func(n *html.Node) (string, bool) {
		if n.Type == html.ElementNode && n.Data == "p" {
			for _, a := range n.Attr {
				if a.Key =="class" && a.Val == "link" {
					if n.FirstChild != nil {
						if u := strings.ToLower(n.FirstChild.Data); strings.HasPrefix(u, "https://") {
							return n.FirstChild.Data, true
						}
					}
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if url, found := fixFunc(c); found {
				return url, true
			}
		}
		return "", false
	}

	if url, found := fixFunc(h); found {
		return url, nil
	}
	return "", fmt.Errorf("cannot fix t.cn not redirecting problem, page source of %v may changed", url)
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

