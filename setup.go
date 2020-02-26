package redirect

import (
	"github.com/caddyserver/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	ups, err := NewReloadableUpstreams(c)
	if err != nil {
		return PluginError(err)
	}

	r := &Redirect{ Upstreams: &ups }
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		r.Next = next
		return r
	})

	c.OnStartup(func() error {
		return r.OnStartup()
	})

	c.OnShutdown(func() error {
		return r.OnShutdown()
	})

	return nil
}

