package redirect

import (
	"github.com/coredns/coredns/plugin"

	"github.com/caddyserver/caddy"
)

const pluginName = "redirect"

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	return nil
}

