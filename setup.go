package dnsredir

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	log.Infof("Initializing, version %v, HEAD %v", pluginVersion, pluginHeadCommit)

	ups, err := NewReloadableUpstreams(c)
	if err != nil {
		return PluginError(err)
	}

	r := &Dnsredir{Upstreams: &ups}
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
