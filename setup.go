package redirect

import (
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/caddyserver/caddy"
)

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	re, err := parseRedirect(c)
	if err != nil {
		return PluginError(err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		re.Next = next
		return re
	})

	return nil
}

func parseRedirect(c *caddy.Controller) (*Redirect, error) {
	var (
		re *Redirect
		err error
		once bool
	)

	for c.Next() {
		if once {
			// Currently, this plugin can only be used once per server block
			return nil, plugin.ErrOnce
		}
		once = true

		re, err = parseRedirectCore(c)
		if err != nil {
			return nil, err
		}
	}

	return re, nil
}

func parseRedirectCore(c *caddy.Controller) (*Redirect, error) {
	re := NewRedirect()

	files := c.RemainingArgs()
	if len(files) == 0 {
		return nil, c.ArgErr()
	}
	log.Infof("files: %v", files)

	return re, nil
}

