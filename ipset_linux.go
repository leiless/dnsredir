// +build linux

package dnsredir

import (
	"github.com/coredns/caddy"
	goipset "github.com/digineo/go-ipset/v2"
	"github.com/miekg/dns"
	"net"
	"os"
	"strings"
)

const (
	nfProtoUnspec = 0
	nfProtoIpv4   = 2
	nfProtoIpv6   = 10
)

type ipsetHandle struct {
	set  StringSet
	conn *goipset.Conn
}

func ipsetParse(c *caddy.Controller, u *reloadableUpstream) error {
	dir := c.Val()
	names := c.RemainingArgs()
	if len(names) == 0 {
		return c.ArgErr()
	}
	if u.ipset == nil {
		u.ipset = &ipsetHandle{
			set: make(StringSet),
		}
	}
	h := u.ipset.(*ipsetHandle)
	for _, name := range names {
		h.set.Add(name)
	}
	log.Infof("%v: %v", dir, names)
	return nil
}

func ipsetSetup(u *reloadableUpstream) (err error) {
	// In case of plugin block doesn't have ipset option, which u.ipset is nil
	// panic: interface conversion: interface {} is nil, not *dnsredir.ipsetHandle
	if u.ipset == nil {
		return nil
	}
	if os.Geteuid() != 0 {
		log.Warningf("ipset needs root user privilege to work")
	}
	ipset := u.ipset.(*ipsetHandle)
	ipset.conn, err = goipset.Dial(nfProtoUnspec, nil)
	if err != nil {
		return err
	}
	return nil
}

func ipsetShutdown(u *reloadableUpstream) (err error) {
	if u.ipset == nil {
		return nil
	}
	return u.ipset.(*ipsetHandle).conn.Close()
}

// Taken from https://github.com/missdeer/ipset/blob/master/reverter.go#L32 with modification
func ipsetAddIP(u *reloadableUpstream, reply *dns.Msg) {
	if u.ipset == nil || reply.Rcode != dns.RcodeSuccess {
		return
	}

	ipset := u.ipset.(*ipsetHandle)
	for _, rr := range reply.Answer {
		rrType := rr.Header().Rrtype
		if rrType != dns.TypeA && rrType != dns.TypeAAAA {
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

		for name := range ipset.set {
			p, err := ipset.conn.Header(name)
			if err != nil {
				log.Errorf("ipsetAddIP(): cannot get ipset %q header: %v", name, err)
				continue
			}

			var typeMatch bool
			if uint(p.Family.Value) == uint(nfProtoIpv4) {
				typeMatch = rrType == dns.TypeA
			} else if uint(p.Family.Value) == uint(nfProtoIpv6) {
				typeMatch = rrType == dns.TypeAAAA
			}
			if !typeMatch {
				continue
			}

			err = ipset.conn.Add(name, goipset.NewEntry(goipset.EntryIP(ip)))
			if err != nil {
				log.Errorf("ipsetAddIP(): cannot add %q to ipset %q: %v", ip, name, err)
			}
		}
	}
}
