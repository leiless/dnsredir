// +build darwin

package dnsredir

import (
	"github.com/coredns/caddy"
	"github.com/leiless/dnsredir/pf"
	"github.com/miekg/dns"
	"net"
	"os"
	"strings"
)

type pfHandle struct {
	set pf.TableSet
	dev int		// File descriptor to the /dev/pf
}

// arg in format of NAME[:ANCHOR]
func splitNameAnchor(arg string) (string, string) {
	i := strings.IndexByte(arg, ':')
	if i < 0 {
		return arg, ""
	}
	return arg[:i], arg[i + 1:]
}

func pfParse(c *caddy.Controller, u *reloadableUpstream) error {
	dir := c.Val()
	list := c.RemainingArgs()
	if len(list) == 0 {
		return c.ArgErr()
	}
	if u.pf == nil {
		u.pf = &pfHandle{
			set: make(pf.TableSet),
			dev: -1,
		}
	}
	handle := u.pf.(*pfHandle)
	for _, arg := range list {
		name, anchor := splitNameAnchor(arg)
		if err := handle.set.Add(name, anchor); err != nil && !os.IsExist(err) {
			return err
		}
	}
	log.Infof("%v: %v", dir, handle.set)
	return nil
}

func pfSetup(u *reloadableUpstream) error {
	if u.pf == nil {
		return nil
	}
	if os.Geteuid() != 0 {
		log.Warningf("pf needs root user privilege to work")
	}
	handle := u.pf.(*pfHandle)
	if dev, err := pf.OpenDevPf(os.O_WRONLY); err != nil {
		return err
	} else {
		handle.dev = dev
		// Try to create the table at pf setup stage.
		for t := range handle.set {
			if created, err := pf.AddTable(handle.dev, t.Name, t.Anchor); err != nil {
				return err
			} else {
				log.Debugf("pf: %v created: %v", t.String(), created)
			}
		}
		return nil
	}
}

func pfShutdown(u *reloadableUpstream) error {
	if u.pf == nil {
		return nil
	}
	handle := u.pf.(*pfHandle)
	return pf.CloseDevPf(handle.dev)
}

func pfAddIP(u *reloadableUpstream, reply *dns.Msg) {
	if u.pf == nil || reply.Rcode != dns.RcodeSuccess {
		return
	}

	handle := u.pf.(*pfHandle)
	for _, rr := range reply.Answer {
		if rrt := rr.Header().Rrtype; rrt != dns.TypeA && rrt != dns.TypeAAAA {
			continue
		}

		ss := strings.Split(rr.String(), "\t")
		if len(ss) != 5 {
			log.Warningf("Expected 5 entries, got %v: %q", len(ss), rr.String())
			continue
		}

		ip := net.ParseIP(ss[4])
		if ip == nil {
			log.Warningf("ipsetAddIP(): %q isn't a valid IP address", ss[4])
			continue
		}

		for t := range handle.set {
			if added, err := pf.AddAddr(handle.dev, t.Name, t.Anchor, ip); err != nil {
				log.Errorf("pf.AddIP(): cannot add %v to %v: %v", ip, t, err)
			} else {
				log.Debugf("pf: %v added: %v", ip.String(), added)
			}
		}
	}
}
