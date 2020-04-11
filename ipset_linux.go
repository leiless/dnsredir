// +build linux

package dnsredir

import (
	"fmt"
	"github.com/caddyserver/caddy"
	goipset "github.com/digineo/go-ipset/v2"
	"github.com/miekg/dns"
	"github.com/ti-mo/netfilter"
	"net"
	"os"
	"strconv"
	"strings"
)

type ipsetHandle struct {
	set [2]map[string]struct{}
	conn *goipset.Conn
}

func parseIpset(c *caddy.Controller, u *reloadableUpstream) error {
	dir := c.Val()
	args := c.RemainingArgs()
	if len(args) <= 1 {
		return c.ArgErr()
	}
	ipType, err := strconv.Atoi(args[0])
	if err != nil {
		return c.Errf("%v: %v", dir, err)
	}
	var i int
	if ipType == 4 {
		i = 0
	} else if ipType == 6 {
		i = 1
	} else {
		return c.Errf("%v: unknown ipset family type: %q", dir, ipType)
	}
	if u.ipset == nil {
		u.ipset = &ipsetHandle{}
	}
	set := u.ipset.(*ipsetHandle).set
	if set[i] == nil {
		set[i] = make(map[string]struct{})
	}
	names := args[1:]
	for _, name := range names {
		set[i][name] = struct{}{}
	}
	log.Infof("%v: IPv%v %v", dir, ipType, names)
	return nil
}

func ipsetSetup(u *reloadableUpstream) (err error) {
	// In case of plugin block doesn't have ipset option, which u.ipset is nil
	// panic: interface conversion: interface {} is nil, not *dnsredir.ipsetHandle
	if u.ipset == nil {
		return nil
	}
	ipset := u.ipset.(*ipsetHandle)
	ipset.conn, err = goipset.Dial(netfilter.ProtoUnspec, nil)
	if err != nil {
		return err
	}
	if os.Geteuid() != 0 {
		log.Warningf("ipset needs root user privilege to work")
	}
	return nil
}

func ipsetShutdown(u *reloadableUpstream) (err error) {
	if u.ipset == nil {
		return nil
	}
	err = u.ipset.(*ipsetHandle).conn.Close()
	if err != nil {
		return err
	}
	return nil
}

// Taken from https://github.com/missdeer/ipset/blob/master/reverter.go#L32 with modification
func ipsetAddIP(u *reloadableUpstream, reply *dns.Msg) {
	if u.ipset == nil {
		return
	}
	ipset := u.ipset.(*ipsetHandle)
	for _, rr := range reply.Answer {
		if rr.Header().Rrtype != dns.TypeA && rr.Header().Rrtype != dns.TypeAAAA {
			continue
		}

		ss := strings.Split(rr.String(), "\t")
		if len(ss) != 5 {
			log.Warningf("Expected 5 entries, got %v: %q", len(ss), rr.String())
			continue
		}

		ip := net.ParseIP(ss[4])
		if ip == nil {
			log.Warningf("addToIpset(): %q isn't a valid IP address", ss[4])
			continue
		}

		var i int
		if ip.To4() != nil {
			i = 0
		} else {
			if ip.To16() == nil {
				panic(fmt.Sprintf("Why %q isn't a valid IPv6 address?!", ip))
			}
			i = 1
		}
		for name := range ipset.set[i] {
			err := ipset.conn.Add(name, goipset.NewEntry(goipset.EntryIP(ip)))
			if err != nil {
				log.Errorf("addToIpset(): error occurred when adding %q: %v", ip, err)
			}
		}
	}
}

