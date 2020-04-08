/*
 * Created Feb 16, 2020
 */

package dnsredir

import (
	"context"
	"errors"
	"fmt"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/debug"
	"github.com/coredns/coredns/plugin/metrics"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	goipset "github.com/digineo/go-ipset/v2"
	"github.com/miekg/dns"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var log = clog.NewWithPlugin(pluginName)

type Dnsredir struct {
	Next plugin.Handler

	Upstreams *[]Upstream
}

// Upstream manages a pool of proxy upstream hosts
// see: github.com/coredns/proxy#proxy.go
type Upstream interface {
	// Check if given domain name should be routed to this upstream zone
	Match(name string) bool
	// Select an upstream host to be routed to, nil if no available host
	Select() *UpstreamHost

	// Exchanger returns the exchanger to be used for this upstream
	//Exchanger() interface{}
	// Send question to upstream host and await for response
	//Exchange(ctx context.Context, state request.Request) (*dns.Msg, error)

	Start() error
	Stop() error
}

func (r *Dnsredir) OnStartup() error {
	for _, up := range *r.Upstreams {
		if err := up.Start(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Dnsredir) OnShutdown() error {
	for _, up := range *r.Upstreams {
		if err := up.Stop(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Dnsredir) ServeDNS(ctx context.Context, w dns.ResponseWriter, req *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: req}
	name := state.Name()

	server := metrics.WithServer(ctx)
	upstream0, t := r.match(server, name)
	if upstream0 == nil {
		log.Debugf("%q not found in name list, t: %v", name, t)
		return plugin.NextOrFailure(r.Name(), r.Next, ctx, w, req)
	}
	upstream := upstream0.(*reloadableUpstream)
	log.Debugf("%q in name list, t: %v", name, t)

	var reply *dns.Msg
	var upstreamErr error
	deadline := time.Now().Add(defaultTimeout)
	for time.Now().Before(deadline) {
		start := time.Now()

		host := upstream.Select()
		if host == nil {
			log.Debug(errNoHealthy)
			return dns.RcodeServerFailure, errNoHealthy
		}
		log.Debugf("Upstream host %v is selected", host.Name())

		for {
			t := time.Now()
			reply, upstreamErr = host.Exchange(ctx, state, upstream.bootstrap)
			log.Debugf("rtt: %v", time.Since(t))
			if upstreamErr == errCachedConnClosed {
				// [sic] Remote side closed conn, can only happen with TCP.
				// Retry for another connection
				log.Debugf("%v: %v", upstreamErr, host.Name())
				continue
			}
			break
		}

		if upstreamErr != nil {
			if upstream.maxFails != 0 {
				log.Warningf("Exchange() failed  error: %v", upstreamErr)
				healthCheck(upstream, host)
			}
			continue
		}

		if !state.Match(reply) {
			debug.Hexdumpf(reply, "Wrong reply  id: %v, qname: %v qtype: %v", reply.Id, state.QName(), state.QType())

			formerr := new(dns.Msg)
			formerr.SetRcode(state.Req, dns.RcodeFormatError)
			_ = w.WriteMsg(formerr)
			return 0, nil
		}

		_ = w.WriteMsg(reply)
		addToIpset(upstream, reply)

		RequestDuration.WithLabelValues(server, host.Name()).Observe(float64(time.Since(start).Milliseconds()))
		RequestCount.WithLabelValues(server, host.Name()).Inc()

		rc, ok := dns.RcodeToString[reply.Rcode]
		if !ok {
			rc = strconv.Itoa(reply.Rcode)
		}
		RcodeCount.WithLabelValues(server, host.Name(), rc).Inc()
		return 0, nil
	}

	if upstreamErr == nil {
		panic("Why upstreamErr is nil?! Are you in a debugger or your machine running slow?")
	}
	return dns.RcodeServerFailure, upstreamErr
}

func healthCheck(r *reloadableUpstream, uh *UpstreamHost) {
	// Skip unnecessary health checking
	if r.checkInterval == 0 || r.maxFails == 0 {
		return
	}

	failTimeout := defaultFailTimeout
	fails := atomic.AddInt32(&uh.fails, 1)
	go func(uh *UpstreamHost) {
		time.Sleep(failTimeout)
		// Failure count may go negative here, should be rectified by HC eventually
		atomic.AddInt32(&uh.fails, -1)
		// Kick off health check on every failureCheck failure
		if fails % failureCheck == 0 {
			_ = uh.Check()
		}
	}(uh)
}

// Taken from https://github.com/missdeer/ipset/blob/master/reverter.go#L32 with modification
func addToIpset(r *reloadableUpstream, reply *dns.Msg) {
	if len(r.ipset[0]) == 0 && len(r.ipset[1]) == 0 {
		return
	}

	for _, rr := range reply.Answer {
		if rr.Header().Rrtype != dns.TypeA && rr.Header().Rrtype != dns.TypeAAAA {
			continue
		}

		ss := strings.Split(rr.String(), "\t")
		if len(ss) != 5 {
			log.Warningf("Expected 5 entries, got %v: %q", len(ss), rr.String())
			continue
		}

		ip := net.ParseIP(ss[4])
		if ip == nil {
			log.Warningf("addToIpset(): %q isn't a valid IP address", ss[4])
			continue
		}

		var i int
		if ip.To4() != nil {
			i = 0
		} else {
			if ip.To16() == nil {
				panic(fmt.Sprintf("Why %q isn't a valid IPv6 address?!", ip))
			}
			i = 1
		}
		for name := range r.ipset[i] {
			err := r.ipsetConn.Add(name, goipset.NewEntry(goipset.EntryIP(ip)))
			if err != nil {
				log.Errorf("addToIpset(): error occurred when adding %q: %v", ip, err)
			}
		}
	}
}

func (r *Dnsredir) Name() string { return pluginName }

func (r *Dnsredir) match(server, name string) (Upstream, time.Duration) {
	t1 := time.Now()

	if r.Upstreams == nil {
		panic("Why Dnsredir.Upstreams is nil?!")
	}

	// Don't check validity of domain name, delegate to upstream host
	if len(name) > 1 {
		name = removeTrailingDot(name)
	}

	for _, up := range *r.Upstreams {
		// For maximum performance, we search the first matched item and return directly
		// Unlike proxy plugin, which try to find longest match
		if up.Match(name) {
			t2 := time.Since(t1)
			NameLookupDuration.WithLabelValues(server, "1").Observe(float64(t2.Milliseconds()))
			return up, t2
		}
	}

	t2 := time.Since(t1)
	NameLookupDuration.WithLabelValues(server, "0").Observe(float64(t2.Milliseconds()))
	return nil, t2
}

var (
	errNoHealthy = errors.New("no healthy upstream host")
	errCachedConnClosed = errors.New("cached connection was closed by peer")
)

const (
	defaultTimeout = 15 * time.Second
	defaultFailTimeout = 2000 * time.Millisecond
	failureCheck = 3
)

