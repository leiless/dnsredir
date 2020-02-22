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

	*Namelist
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
// Lookup divided into two steps
//	1. Fast lookup
//	2. Full lookup
func (re *Redirect) match(name string) bool {
	// TODO: Add a metric value in Prometheus to determine average lookup time

	child := RemoveTrailingDot(name)

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

