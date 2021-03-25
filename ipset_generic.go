// +build !linux

package dnsredir

import (
	"github.com/coredns/caddy"
	"github.com/miekg/dns"
	"runtime"
)

var ipsetOnce Once

func ipsetParse(c *caddy.Controller, u *reloadableUpstream) error {
	_ = u
	ipsetOnce.Do(func() {
		dir := c.Val()
		// #9 Consume remaining arguments to fix Corefile parse error
		_ = c.RemainingArgs()
		log.Warningf("%v is not available on %v", dir, runtime.GOOS)
	})
	return nil
}

func ipsetSetup(u *reloadableUpstream) error {
	_ = u
	return nil
}

func ipsetShutdown(u *reloadableUpstream) error {
	_ = u
	return nil
}

func ipsetAddIP(r *reloadableUpstream, reply *dns.Msg) {
	_, _ = r, reply
}
