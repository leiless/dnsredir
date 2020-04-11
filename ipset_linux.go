// +build linux

package dnsredir

import (
	"fmt"
	goipset "github.com/digineo/go-ipset/v2"
	"github.com/miekg/dns"
	"github.com/ti-mo/netfilter"
	"net"
	"os"
	"strings"
)

func ipsetSetup(u *reloadableUpstream) (err error) {
	if len(u.ipset[0]) != 0 || len(u.ipset[1]) != 0 {
		u.ipsetConn, err = goipset.Dial(netfilter.ProtoUnspec, nil)
		if err != nil {
			return err
		}
		if os.Geteuid() != 0 {
			log.Warningf("ipset needs root user privilege to work")
		}
	}
	return nil
}

func ipsetShutdown(u *reloadableUpstream) (err error) {
	if len(u.ipset[0]) != 0 || len(u.ipset[1]) != 0 {
		err = u.ipsetConn.(*goipset.Conn).Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// Taken from https://github.com/missdeer/ipset/blob/master/reverter.go#L32 with modification
func ipsetAddIP(r *reloadableUpstream, reply *dns.Msg) {
	if len(r.ipset[0]) == 0 && len(r.ipset[1]) == 0 {
		return
	}

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
		for name := range r.ipset[i] {
			err := r.ipsetConn.(*goipset.Conn).Add(name, goipset.NewEntry(goipset.EntryIP(ip)))
			if err != nil {
				log.Errorf("addToIpset(): error occurred when adding %q: %v", ip, err)
			}
		}
	}
}

