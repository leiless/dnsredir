/*
 * Created Feb 16, 2020
 */

package dnsredir

import (
	"context"
	"errors"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/debug"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
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

	upstream, t := r.match(name)
	if upstream == nil {
		log.Debugf("%q not found in name list, t: %v", name, t)
		return plugin.NextOrFailure(r.Name(), r.Next, ctx, w, req)
	}
	log.Debugf("%q in name list, t: %v", name, t)

	var reply *dns.Msg
	var upstreamErr error
	deadline := time.Now().Add(defaultTimeout)
	for time.Now().Before(deadline) {
		host := upstream.Select()
		if host == nil {
			log.Debug(errNoHealthy)
			return dns.RcodeServerFailure, errNoHealthy
		}
		log.Debugf("Upstream host %v is selected", host.addr)

		for {
			t := time.Now()
			reply, upstreamErr = host.Exchange(ctx, state)
			log.Debugf("rtt: %v", time.Since(t))
			if upstreamErr == errCachedConnClosed {
				// [sic] Remote side closed conn, can only happen with TCP.
				// Retry for another connection
				log.Debugf("%v: %v", upstreamErr, host.addr)
				continue
			}
			if reply != nil && reply.Truncated && !host.transport.forceTcp && host.transport.preferUdp {
				log.Warningf("TODO: Retry with TCP since response truncated and prefer_udp configured")
			}
			break
		}

		if upstreamErr != nil {
			log.Warningf("Exchange() failed  error: %v", upstreamErr)
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
		return 0, nil
	}

	if upstreamErr == nil {
		panic("Why upstreamErr is nil?! Are you in a debugger or your machine running slow?")
	}
	return dns.RcodeServerFailure, upstreamErr
}

func (r *Dnsredir) Name() string { return pluginName }

func (r *Dnsredir) match(name string) (Upstream, time.Duration) {
	if r.Upstreams == nil {
		panic("Why Dnsredir.Upstreams is nil?!")
	}

	// TODO: Add a metric value in Prometheus to determine average lookup time

	t := time.Now()
	for _, up := range *r.Upstreams {
		// Q: perform longest domain match?
		// For maximum performance, we search the first matched item and return directly
		// Unlike proxy plugin, which try to find longest match
		if up.Match(name) {
			return up, time.Since(t)
		}
	}

	return nil, time.Since(t)
}

var (
	errNoHealthy = errors.New("no healthy upstream host")
	errCachedConnClosed = errors.New("cached connection was closed by peer")
)

const defaultTimeout = 15000 * time.Millisecond

