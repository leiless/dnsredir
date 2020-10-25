// +build darwin

package pf

import (
	"github.com/coredns/caddy"
	"github.com/leiless/dnsredir"
	"github.com/miekg/dns"
	"net"
	"os"
	"strings"
)

type pfHandle struct {
	set tableSet
	dev int		// File descriptor to the /dev/pf
}

var log = dnsredir.Log

// arg in format of NAME[:ANCHOR]
func splitNameAnchor(arg string) (string, string) {
	i := strings.IndexByte(arg, ':')
	if i < 0 {
		return arg, ""
	}
	return arg[:i], arg[i + 1:]
}

func Parse(c *caddy.Controller, u *dnsredir.ReloadableUpstream) error {
	dir := c.Val()
	list := c.RemainingArgs()
	if len(list) == 0 {
		return c.ArgErr()
	}
	if u.Pf == nil {
		u.Pf = &pfHandle{
			set: make(tableSet),
		}
	}
	pf := u.Pf.(*pfHandle)
	for _, arg := range list {
		name, anchor := splitNameAnchor(arg)
		if err := pf.set.Add(name, anchor); err != nil && !os.IsExist(err) {
			return err
		}
	}
	log.Infof("%v: %v", dir, pf.set)
	return nil
}

func Setup(u *dnsredir.ReloadableUpstream) error {
	if u.Pf == nil {
		return nil
	}
	if os.Geteuid() != 0 {
		log.Warningf("pf needs root user privilege to work")
	}
	pf := u.Pf.(*pfHandle)
	if dev, err := openDevPf(os.O_WRONLY); err != nil {
		return err
	} else {
		pf.dev = dev
		return nil
	}
}

func Shutdown(u *dnsredir.ReloadableUpstream) error {
	if u.Pf == nil {
		return nil
	}
	pf := u.Pf.(*pfHandle)
	return closeDevPf(pf.dev)
}

func AddIP(u *dnsredir.ReloadableUpstream, reply *dns.Msg) {
	if u.Pf == nil || reply.Rcode != dns.RcodeSuccess {
		return
	}
	pf := u.Pf.(*pfHandle)
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

		for t := range pf.set {
			_ = t
		}
	}
}
