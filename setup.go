package redirect

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/caddyserver/caddy"
)

func init() { plugin.Register(pluginName, Setup) }

func Setup(c *caddy.Controller) error {
	re, err := ParseRedirect(c)
	if err != nil {
		return PluginError(err)
	}

	c.OnStartup(func() error {
		re.parseNamelist()
		return nil
	})

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		re.Next = next
		return re
	})

	return nil
}

func ParseRedirect(c *caddy.Controller) (*Redirect, error) {
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

		re, err = ParseRedirect2(c)
		if err != nil {
			return nil, err
		}
	}

	return re, nil
}

func ParseRedirect2(c *caddy.Controller) (*Redirect, error) {
	config := dnsserver.GetConfig(c)
	re := NewRedirect()

	paths := c.RemainingArgs()
	if len(paths) == 0 {
		return nil, c.ArgErr()
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) && config.Root != "" {
			path = filepath.Join(config.Root, path)
		}

		s, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				log.Warningf("File %s doesn't exist", path)
			} else {
				return nil, err
			}
		} else if s != nil && !s.Mode().IsRegular() {
			log.Warningf("File %s isn't a regular file")
		}
	}
	re.items = PathsToNameitems(paths)
	log.Debugf("Files: %v", paths)

	for c.NextBlock() {
		if err := ParseBlock(c, re); err != nil {
			return nil, err
		}
	}

	return re, nil
}

func ParseBlock(c *caddy.Controller, re *Redirect) error {
	switch c.Val() {
	case "reload":
		args := c.RemainingArgs()
		if len(args) != 1 {
			return c.ArgErr()
		}
		arg := args[0]
		if _, e := strconv.Atoi(arg); e == nil {
			log.Warningf("reload %s missing time unit, assume it's second", arg)
			arg += "s"
		}
		d, err := time.ParseDuration(arg)
		if err != nil {
			return err
		}
		if d < 0 {
			return c.Errf("negative time duration: %s", args[0])
		}
		re.reload = d
		log.Debugf("Reload time duration: %v", d)
	default:
		return c.Errf("unknown directive: %s", c.Val())
	}
	return nil
}

