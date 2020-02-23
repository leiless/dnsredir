/*
 * Created Feb 16, 2020
 */

package redirect

import (
	"context"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/whoami"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
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
	Select() interface{}
	// Exchanger returns the exchanger to be used for this upstream
	Exchanger() interface{}

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

	if !r.match(name) {
		log.Debugf("%v not found in name list", name)
		return plugin.NextOrFailure(r.Name(), r.Next, ctx, w, req)
	}

	log.Debugf("%v in name list", name)
	return whoami.Whoami{}.ServeDNS(ctx, w, req)
}

func (r *Redirect) Name() string { return pluginName }

func (r *Redirect) match(name string) bool {
	// TODO: Add a metric value in Prometheus to determine average lookup time

	for _, up := range *r.Upstreams {
		// TODO: perform longest prefix match?
		if up.Match(name) {
			return true
		}
	}

	return false
}

