/*
 * Created Feb 16, 2020
 */

package redirect

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

type Redirect struct {
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

func (r *Redirect) OnStartup() error {
	for _, up := range *r.Upstreams {
		if err := up.Start(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Redirect) OnShutdown() error {
	for _, up := range *r.Upstreams {
		if err := up.Stop(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Redirect) ServeDNS(ctx context.Context, w dns.ResponseWriter, req *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: req}
	name := state.Name()

	upstream := r.match(name)
	if upstream == nil {
		log.Debugf("%v not found in name list", name)
		return plugin.NextOrFailure(r.Name(), r.Next, ctx, w, req)
	}
	log.Debugf("%v in name list", name)

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

		reply, upstreamErr = host.Exchange(ctx, state)
		if upstreamErr == nil {
			if !state.Match(reply) {
				debug.Hexdumpf(reply, "Wrong reply  id: %d, qname: %s qtype: %d", reply.Id, state.QName(), state.QType())

				formerr := new(dns.Msg)
				formerr.SetRcode(state.Req, dns.RcodeFormatError)
				_ = w.WriteMsg(formerr)
				return 0, nil
			}

			_ = w.WriteMsg(reply)
			return 0, nil
		}
	}

	if upstreamErr == nil {
		panic("Why upstreamErr is nil?! Are you in a debugger?")
	}
	return dns.RcodeServerFailure, upstreamErr

	// redirect-whoami for DEBUGGING
	//return whoami.Whoami{}.ServeDNS(ctx, w, req)
}

func (r *Redirect) Name() string { return pluginName }

func (r *Redirect) match(name string) Upstream {
	if r.Upstreams == nil {
		log.Warningf("redirect have no upstream hosts at all")
		return nil
	}

	// TODO: Add a metric value in Prometheus to determine average lookup time

	for _, up := range *r.Upstreams {
		// TODO: perform longest prefix match?
		if up.Match(name) {
			return up
		}
	}

	return nil
}

var (
	errNoHealthy = errors.New("no healthy upstream host")
)

//const defaultTimeout = 5 * time.Second
const defaultTimeout = 500 * time.Millisecond

