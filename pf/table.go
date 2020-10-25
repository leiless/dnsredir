package pf

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type table struct {
	name string
	anchor string		// anchor can be empty
}

// XXX: not thread safe
type tableSet map[table]struct{}

// see: XNU #include <net/pfvar.h>
const (
	maxTableNameSize  = 32
	maxAnchorNameSize = 1024
)

func (s *tableSet) Add(name string, anchorArg ...string) error {
	if name == "" || len(name) >= maxTableNameSize {
		return errors.New("table name is empty or too long")
	}
	if len(anchorArg) > 1 {
		return errors.New("at most one anchor name can be specified")
	}
	anchor := ""
	if len(anchorArg) != 0 {
		if anchor = anchorArg[0]; len(anchor) >= maxAnchorNameSize {
			return errors.New("anchor name is too long")
		}
	}
	t := table{
		name:   name,
		anchor: anchor,
	}
	if _, found := (*s)[t]; found {
		return os.ErrExist
	}
	(*s)[t] = struct{}{}
	return nil
}

func (s *tableSet) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%T ", *s))
	i := 0
	n := len(*s)
	for t := range *s {
		if t.anchor != "" {
			sb.WriteString(fmt.Sprintf("%v:%v", t.name, t.anchor))
		} else {
			sb.WriteString(t.name)
		}
		if i++; i != n {
			sb.WriteByte(' ')
		}
	}
	sb.WriteByte(']')
	return sb.String()
}
