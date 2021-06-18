// +build darwin

package pf

import (
	"fmt"
	"os"
	"strings"
)

type Flags uint

const (
	tableFlagCreateIfNotExist = Flags(1 << iota)
	tableFlagIPv4Only
	tableFlagIPv6Only
	tableFlagLast
)

func (f Flags) IsValid() bool {
	if f.IsV4Only() && f.IsV6Only() {
		return false
	}
	return f & ^(tableFlagLast-1) == 0
}

func (f *Flags) TurnOnCreateIfNotExist() {
	*f |= tableFlagCreateIfNotExist
}

func (f *Flags) TurnOnV4Only() {
	*f |= tableFlagIPv4Only
}

func (f *Flags) TurnOnV6Only() {
	*f |= tableFlagIPv6Only
}

func (f Flags) IsCreateIfNotExist() bool {
	return (f & tableFlagCreateIfNotExist) != 0
}

func (f Flags) IsV4Only() bool {
	return (f & tableFlagIPv4Only) != 0
}

func (f Flags) IsV6Only() bool {
	return (f & tableFlagIPv6Only) != 0
}

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
type TableSet map[table]Flags

// see: XNU #include <net/pfvar.h>
const (
	maxTableNameSize  = 32
	maxAnchorNameSize = 1024
)

func (s *TableSet) Add(name, anchor string, flags Flags) error {
	if name == "" || len(name) >= maxTableNameSize {
		return fmt.Errorf("table name is empty or too long")
	}
	if len(anchor) >= maxAnchorNameSize {
		return fmt.Errorf("anchor name is too long")
	}
	t := table{
		Name:   name,
		Anchor: anchor,
	}
	if _, found := (*s)[t]; found {
		return os.ErrExist
	}
	(*s)[t] = flags
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
