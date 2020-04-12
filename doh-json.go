/*
 * Code taken from github.com/m13253/dns-over-https/doh-client/google.go with modification
 */

package dnsredir

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/coredns/coredns/request"
	"github.com/m13253/dns-over-https/json-dns"
	"github.com/miekg/dns"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (uh *UpstreamHost) jsonDnsExchange(ctx context.Context, state request.Request, bootstrap []string) (*dns.Msg, error) {
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
		uh.Name(), uh.requestContentType, url.QueryEscape(q.Name), url.QueryEscape(QType))
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
	req.Header.Set("Accept", "application/dns-json, application/json")
	userAgent := fmt.Sprintf("coredns-%v %v %v", pluginName, pluginVersion, pluginVersion)
	req.Header.Set("User-Agent", userAgent)

	resp, err := uh.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer Close(resp.Body)

	contentType := strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0]
	switch contentType {
	case "application/dns-json":
		fallthrough
	case "application/json":
		return uh.jsonDnsParseResponse(state, resp, contentType)
	case "application/dns-message":
		// TODO
	default:
		log.Warningf("Met unknown Content-Type: %q", contentType)
		switch uh.requestContentType {
		case "application/dns-json":
			return uh.jsonDnsParseResponse(state, resp, contentType)
		case "application/dns-message":
			// TODO
		default:
			panic(fmt.Sprintf("Unknown request Content-Type: %q", uh.requestContentType))
		}
	}
	panic("TODO: NYI")
}

func (uh *UpstreamHost) jsonDnsParseResponse(state request.Request, resp *http.Response, contentType string) (*dns.Msg, error) {
	if resp.StatusCode != http.StatusOK {
		if contentType != uh.requestContentType {
			return nil, fmt.Errorf("upstream %v error: bad status: %v content type: %v",
				uh.Name(), resp.StatusCode, contentType)
		}
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var respJSON jsonDNS.Response
	err = json.Unmarshal(body, &respJSON)
	if err != nil {
		return nil, err
	}
	if respJSON.Status != dns.RcodeSuccess && respJSON.Comment != "" {
		log.Warningf("DNS error when query %q: %v", state.Name(), respJSON.Comment)
	}

	udpSize := state.Size()
	if udpSize < dns.MinMsgSize {
		udpSize = dns.MinMsgSize
	}
	reply := jsonDNS.PrepareReply(state.Req)
	reply = jsonDNS.Unmarshal(reply, &respJSON, uint16(udpSize), 0)
	return reply, nil
}

