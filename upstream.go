/*
 * Created Feb 23, 2020
 */

package redirect

import (
	"context"
	"github.com/caddyserver/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type reloadableUpstream struct {
	*Namelist
	ignored domainSet
	*HealthCheck
}

// reloadableUpstream implements Upstream interface

// Check if given name in upstream name list
func (u *reloadableUpstream) Match(name string) bool {
	child, ok := stringToDomain(name)
	if !ok {
		log.Warningf("Skip invalid domain '%v', report to Github repo if it's an error.", name)
		return false
	}

	// The ignored domain map should be relatively small
	if u.ignored.Match(child) {
		return false
	}

	if u.Namelist.Match(child) {
		return true
	}

	return false
}

func (u *reloadableUpstream) Exchange(ctx context.Context, state request.Request) (*dns.Msg, error) {
	// TODO
	return nil, nil
}

func (u *reloadableUpstream) Start() error {
	u.periodicUpdate()
	return nil
}

func (u *reloadableUpstream) Stop() error {
	close(u.stopUpdateChan)
	return nil
}

// Parses Caddy config input and return a list of reloadable upstream for this plugin
func NewReloadableUpstreams(c *caddy.Controller) ([]Upstream, error) {
	var ups []Upstream

	for c.Next() {
		u, err := newReloadableUpstream(c)
		if err != nil {
			return nil, err
		}
		ups = append(ups, u)
	}

	return ups, nil
}

func newReloadableUpstream(c *caddy.Controller) (Upstream, error) {
	config := dnsserver.GetConfig(c)

	u := &reloadableUpstream{
		Namelist: &Namelist{
			reload: defaultReloadDuration,
		},
	}

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
			log.Warningf("File %s isn't a regular file", path)
		}
	}
	u.items = NewNameitemsWithPaths(paths)
	log.Infof("Files: %v", paths)

	for c.NextBlock() {
		if err := parseBlock(c, u); err != nil {
			return nil, err
		}
	}

	return u, nil
}

func parseBlock(c *caddy.Controller, u *reloadableUpstream) error {
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
		u.reload = d
		log.Infof("Reload time duration: %v", d)
	case "except":
		ignored := c.RemainingArgs()
		if len(ignored) == 0 {
			return c.ArgErr()
		}
		u.ignored = make(domainSet)
		for _, name := range ignored {
			if !u.ignored.Add(name) {
				log.Warningf("'%v' isn't a domain name", name)
			}
		}
		log.Infof("ignored: %v", u.ignored)
	default:
		return c.Errf("unknown directive: %s", c.Val())
	}
	return nil
}

const defaultReloadDuration = 5 * time.Second

