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
	"time"
)

var log = clog.NewWithPlugin(pluginName)

type Redirect struct {
	Next plugin.Handler

	Upstreams *[]Upstream

	*Namelist
	ignored stringSet
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

func NewRedirect() *Redirect {
	return &Redirect{
		Namelist: &Namelist{
			reload: 5 * time.Second,
		},
	}
}

func (re *Redirect) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	name := state.Name()

	if !re.match(name) {
		log.Debugf("%v not found in namelist", name)
		return plugin.NextOrFailure(re.Name(), re.Next, ctx, w, r)
	}

	log.Debugf("%v in namelist", name)
	return whoami.Whoami{}.ServeDNS(ctx, w, r)
}

func (re *Redirect) Name() string { return pluginName }

// Check if given name in Redirect namelist
// Lookup divided into three steps
// 	1. Ignored lookup
//	2. Fast lookup
//	3. Full lookup
func (re *Redirect) match(name string) bool {
	// TODO: Add a metric value in Prometheus to determine average lookup time

	child, ok := stringToDomain(name)
	if !ok {
		// TODO: Tell user to report this error if it's a valid domain?
		log.Warningf("'%v' isn't a valid domain name?", name)
		return false
	}

	// The ignored domain map should be relatively small
	for parent := range re.ignored {
		if plugin.Name(parent).Matches(child) {
			log.Debugf("'%v' is ignored", child)
			return false
		}
	}

	// Fast lookup for a full match
	for _, item := range re.items {
		item.RLock()
		if _, ok := item.names[child]; ok {
			item.RUnlock()
			return true
		}
		item.RUnlock()
	}

	// Fallback to iterate the whole namelist
	for _, item := range re.items {
		item.RLock()
		for parent := range item.names {
			if plugin.Name(parent).Matches(child) {
				item.RUnlock()
				return true
			}
		}
		item.RUnlock()
	}

	return false
}

