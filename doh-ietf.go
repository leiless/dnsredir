/*
 * Code taken from github.com/m13253/dns-over-https/doh-client/ietf.go with modification
 */

package dnsredir

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

func (uh *UpstreamHost) ietfDnsExchange(ctx context.Context, state *request.Request, requestContentType string) (*http.Response, error) {
	r := state.Req
	reqId := r.Id
	// [sic]
	//	In order to maximize HTTP cache friendliness, DoH clients using media
	//	formats that include the ID field from the DNS message header, such as
	//	"application/dns-message", SHOULD use a DNS ID of 0 in every DNS request.
	// see: https://tools.ietf.org/html/rfc8484#section-4.1
	r.Id = 0
	reqBytes, err := r.Pack()
	if err != nil {
		return nil, err
	}
	r.Id = reqId

	reqBase64 := base64.RawURLEncoding.EncodeToString(reqBytes)
	reqURL := fmt.Sprintf("%v?ct=%v&dns=%v", uh.Name(), requestContentType, reqBase64)

	var req *http.Request
	// see:
	//	https://technomanor.wordpress.com/2012/04/03/maximum-url-size/
	//	http://archive.is/wOsUj
	if len(reqURL) < 2048 {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	} else {
		// [sic]
		//	When using the POST method, the data payload for this media type MUST
		//	NOT be encoded and is used directly as the HTTP message body.
		// https://tools.ietf.org/html/rfc8484#section-6
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(reqBytes))
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", headerAccept)
	req.Header.Set("User-Agent", userAgent)
	return uh.httpClient.Do(req)
}

func (uh *UpstreamHost) ietfDnsParseResponse(state *request.Request, resp *http.Response, contentType string, requestContentType string) (*dns.Msg, error) {
	if resp.StatusCode != http.StatusOK {
		if contentType != requestContentType {
			return nil, fmt.Errorf("upstream %v error: bad status: %v content type: %v",
				uh.Name(), resp.StatusCode, contentType)
		}
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Unlike ietf.go#parseResponseIETF(), we won't try to rectify TTLs due to networking latency.
	//	since longest latency difference is less than 10 seconds, which tolerant for daily usage.
	// Since we don't want to introduce too many complexities over this CoreDNS plugin.
	reply := new(dns.Msg)
	if err := reply.Unpack(body); err != nil {
		return nil, err
	}
	if reply.Id == 0 {
		// Correct previously zeroed-out DNS request ID
		reply.Id = state.Req.Id
	}
	return reply, nil
}
