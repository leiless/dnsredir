// +build !darwin

// pf is generally available in BSD-derived systems,
//	yet currently we do not have plan to support other BSD distributions than macOS.

package dnsredir

import (
	"github.com/coredns/caddy"
	"github.com/miekg/dns"
	"runtime"
)

var pfOnce Once

func pfParse(c *caddy.Controller, u *reloadableUpstream) error {
	_ = u
	dir := c.Val()
	// #9 Consume remaining arguments to fix Corefile parse error
	_ = c.RemainingArgs()
	pfOnce.Do(func() {
		log.Warningf("%v is not available on %v", dir, runtime.GOOS)
	})
	return nil
}

func pfSetup(u *reloadableUpstream) error {
	_ = u
	return nil
}

func pfShutdown(u *reloadableUpstream) error {
	_ = u
	return nil
}

func pfAddIP(r *reloadableUpstream, reply *dns.Msg) {
	_, _ = r, reply
}
