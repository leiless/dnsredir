// +build darwin

package dnsredir

import (
	"fmt"
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

func parseFlags(dir string, args *[]string) (pf.Flags, error) {
	var flags pf.Flags
	var i int
	var arg string
	for i, arg = range *args {
		if strings.HasPrefix(arg, "+") {
			switch arg {
			case "+create":
				flags.TurnOnCreateIfNotExist()
			case "+v4_only":
				flags.TurnOnV4Only()
			case "+v6_only":
				flags.TurnOnV6Only()
			default:
				return 0, fmt.Errorf("%v: unrecognizable option: %q", dir, arg)
			}
		} else {
			break
		}
	}
	if strings.HasPrefix(arg, "+") {
		return 0, fmt.Errorf("%v: name[:anchor]... not found in %q", dir, strings.Join(*args, " "))
	}
	if !flags.IsValid() {
		return 0, fmt.Errorf("%v: met invalid/conflict options in %q", dir, strings.Join(*args, " "))
	}
	*args = (*args)[i:]
	return flags, nil
}

func pfParse(c *caddy.Controller, u *reloadableUpstream) error {
	dir := c.Val()
	args := c.RemainingArgs()
	if len(args) == 0 {
		return c.ArgErr()
	}
	if u.pf == nil {
		u.pf = &pfHandle{
			set: make(pf.TableSet),
			dev: -1,
		}
	}
	flags, err := parseFlags(dir, &args)
	if err != nil {
		return err
	}
	handle := u.pf.(*pfHandle)
	for _, arg := range args {
		name, anchor := splitNameAnchor(arg)
		if err := handle.set.Add(name, anchor, flags); err != nil && !os.IsExist(err) {
			return err
		}
	}
	log.Infof("%v: %v", dir, handle.set.String())
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
		// Try to create tables at pf setup stage.
		for tbl, flags := range handle.set {
			if !flags.IsCreateIfNotExist() {
				continue
			}
			if created, err := pf.AddTable(handle.dev, tbl.Name, tbl.Anchor); err != nil {
				return err
			} else {
				log.Debugf("pf: %v created: %v", tbl.String(), created)
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
		rrt := rr.Header().Rrtype
		if rrt != dns.TypeA && rrt != dns.TypeAAAA {
			continue
		}

		ss := strings.Split(rr.String(), "\t")
		if len(ss) != 5 {
			log.Warningf("Expected 5 entries, got %v: %q", len(ss), rr.String())
			continue
		}

		ip := net.ParseIP(ss[4])
		if ip == nil {
			log.Warningf("ipsetAddIP(): %q is not a valid IP address", ss[4])
			continue
		}

		for tbl, flags := range handle.set {
			if flags.IsV4Only() && rrt != dns.TypeA {
				continue
			}
			if flags.IsV6Only() && rrt != dns.TypeAAAA {
				continue
			}

			if added, err := pf.AddAddr(handle.dev, tbl.Name, tbl.Anchor, ip); err != nil {
				log.Errorf("pf.AddIP(): cannot add %v to %v: %v", ip, tbl, err)
			} else {
				log.Debugf("pf: %v added: %v", ip.String(), added)
			}
		}
	}
}
