/*
 * Created Feb 16, 2020
 */

package redirect

import (
	"context"
	"time"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin(pluginName)

type Redirect struct {
	Next plugin.Handler

	files []string

	reload time.Duration
}

func NewRedirect() *Redirect {
	return &Redirect{}
}

func (re *Redirect) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	log.Infof("ServeDNS called...")
	return 0, nil
}

func (re *Redirect) Name() string { return pluginName }

