/*
 * Code taken from github.com/m13253/dns-over-https/doh-client/google.go with modification
 */

package dnsredir

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"github.com/coredns/coredns/request"
	jsondns "github.com/m13253/dns-over-https/v2/json-dns"
	"github.com/miekg/dns"
)

func (uh *UpstreamHost) jsonDnsExchange(ctx context.Context, state *request.Request, requestContentType string) (*http.Response, error) {
	r := state.Req
	if r.Response {
		return nil, fmt.Errorf("received a response packet")
	}

	if len(r.Question) != 1 {
		return nil, fmt.Errorf("JSON DOH only supported one question per query, got %v", len(r.Question))
	}
	q := r.Question[0]
	if q.Qclass != dns.ClassINET {
		var QClass string
		if c, ok := dns.ClassToString[q.Qclass]; ok {
			QClass = c
		} else {
			QClass = strconv.FormatUint(uint64(q.Qclass), 10)
		}
		return nil, fmt.Errorf("only IN question class are supported, got %v", QClass)
	}

	var QType string
	if t, ok := dns.TypeToString[q.Qtype]; ok {
		QType = t
	} else {
		QType = strconv.FormatUint(uint64(q.Qtype), 10)
	}

	reqURL := fmt.Sprintf("%v?ct=%v&name=%v&type=%v",
		uh.Name(), requestContentType, url.QueryEscape(q.Name), url.QueryEscape(QType))
	if r.CheckingDisabled {
		// Disable DNSSEC validation
		reqURL += "&cd=1"
	}
	if opt := r.IsEdns0(); opt != nil {
		if opt.Do() {
			reqURL += "&do=1"
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", headerAccept)
	req.Header.Set("User-Agent", userAgent)
	return uh.httpClient.Do(req)
}

func (uh *UpstreamHost) jsonDnsParseResponse(state *request.Request, resp *http.Response, contentType string, requestContentType string) (*dns.Msg, error) {
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

	var respJSON jsondns.Response
	err = json.Unmarshal(body, &respJSON)
	if err != nil {
		return nil, err
	}
	if respJSON.Status != dns.RcodeSuccess && respJSON.Comment != "" {
		log.Warningf("DNS error when query %q: %v", state.Name(), respJSON.Comment)
	}
	fixEmptyNames(&respJSON)

	var udpSize int
	if state.W != nil {
		udpSize = state.Size()
	} else {
		// see: healthcheck.go#UpstreamHost.dohSend()
		q := state.Req.Question[0]
		if q.Name != "." || q.Qtype != dns.TypeNS {
			panic(fmt.Sprintf("Expected query is \"IN NS .\" when state.W is nil, got %v", q))
		}
	}
	if udpSize < dns.MinMsgSize {
		udpSize = dns.MinMsgSize
	}
	reply := jsondns.PrepareReply(state.Req)
	reply = jsondns.Unmarshal(reply, &respJSON, uint16(udpSize), 0)
	return reply, nil
}

// [#2] Fix DNS response empty []RR.Name in DOH JSON API
// Additional section won't be rectified
// see: https://stackoverflow.com/questions/52136176/what-is-additional-section-in-dns-and-how-it-works
func fixEmptyNames(respJSON *jsondns.Response) {
	for i := range respJSON.Answer {
		if respJSON.Answer[i].Name == "" {
			respJSON.Answer[i].Name = "."
		}
	}
	for i := range respJSON.Authority {
		if respJSON.Authority[i].Name == "" {
			respJSON.Authority[i].Name = "."
		}
	}
}
