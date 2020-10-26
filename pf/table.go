// +build darwin

package pf

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type table struct {
	Name   string
	Anchor string // Anchor can be empty
}

func (t *table) String() string {
	if t.Anchor != "" {
		return fmt.Sprintf("%v:%v", t.Name, t.Anchor)
	}
	return t.Name
}

// XXX: not thread safe
type TableSet map[table]struct{}

// see: XNU #include <net/pfvar.h>
const (
	maxTableNameSize  = 32
	maxAnchorNameSize = 1024
)

func (s *TableSet) Add(name string, anchorArg ...string) error {
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
		Name:   name,
		Anchor: anchor,
	}
	if _, found := (*s)[t]; found {
		return os.ErrExist
	}
	(*s)[t] = struct{}{}
	return nil
}

func (s *TableSet) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%T ", *s))
	i := 0
	n := len(*s)
	for t := range *s {
		if t.Anchor != "" {
			sb.WriteString(fmt.Sprintf("%v:%v", t.Name, t.Anchor))
		} else {
			sb.WriteString(t.Name)
		}
		if i++; i != n {
			sb.WriteByte(' ')
		}
	}
	sb.WriteByte(']')
	return sb.String()
}
