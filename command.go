package main

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	profile = `(.+@.+:\d+)`
	integer = `(\d+)`
	rest    = `(.*)`
	spaces  = `\s+`
)

type command struct {
	cmd        string           // cmd such as "contacts" or "msg"
	helptext   string           // description of command
	args       []argdef         // list of re to try (in order) to parse parameters
	defaultSub string           // name of default subcommand
	subcmds    commanddefs      // subcommand definitions
	fn         func(*parsedcmd) // action function
}

type argdef struct {
	name string
	re   *regexp.Regexp // probably have to change this back to string. 'weirdness' without ^ and $
}

type parsedcmd struct {
	cmd     string
	args    []string
	sub     *parsedcmd
	fn      func(*parsedcmd) // access through 'run()'
	usagefn func(int) string // access through 'useage()'
	err     error
}

type commanddefs map[string]command

func (cmds commanddefs) parse(line string) parsedcmd {
	front, back := split(line)
	c, ok := cmds[front]
	if ok {
		return c.complete(back)
	}
	return parsedcmd{
		cmd:  front,
		args: []string{back},
		err:  fmt.Errorf("not a command"),
	}
}

func (cmds commanddefs) help() string {
	var output string
	for _, c := range cmds {
		output += c.usage(0)
	}
	return output
}

func re(exps ...string) *regexp.Regexp {
	exp := strings.Join(exps, spaces)
	return regexp.MustCompile("^" + exp + "$")
}

func split(line string) (front, back string) {
	line = strings.TrimSpace(line)
	parts := strings.SplitN(line, " ", 2) // parts will always be at least len 1
	if len(parts) <= 1 {
		parts = append(parts, "") // ensure parts is len 2
	}
	return parts[0], strings.TrimSpace(parts[1])
}

// assumes "rest" is trimmed of space
func (c command) complete(rest string) (p parsedcmd) {
	p.cmd = c.cmd
	p.fn = c.fn
	p.usagefn = c.usage

	if len(c.subcmds) > 0 {

		s := c.subcmds.parse(rest)
		if _, ok := c.subcmds[s.cmd]; s.err != nil && !ok { // equal to testing error for 'not a command'
			s = c.subcmds.parse(c.defaultSub + " " + rest)
		}
		p.sub = &s
		p.err = s.err // 'bubble up' error

	} else {

		for _, def := range c.args {
			// matches := re(defs...).FindStringSubmatch(rest)
			matches := def.re.FindStringSubmatch(rest)
			if len(matches) > 0 {
				p.args = matches[1:]
				break
			}
		}
		if len(c.args) > 0 && len(p.args) == 0 {
			if rest == "" {
				p.err = fmt.Errorf("expected arguments")
			} else {
				p.args = []string{rest}
				p.err = fmt.Errorf("incorrect arguments")
			}
		}

	}

	return
}

func (c command) usage(lvl int) string {
	const (
		tab = "\t"
		nl  = "\n"
	)
	prefix := strings.Repeat(tab, lvl) // indent

	var args []string
	for _, def := range c.args {
		args = append(args, def.name)
	}
	argstring := strings.Join(args, "|")

	// command's main two lines
	output := prefix + c.cmd + tab + argstring + nl
	output += prefix + tab + c.helptext + nl

	// subcommand list
	if len(c.subcmds) > 0 {
		var defcmd string
		if c.defaultSub != "" {
			defcmd = " (defaults to " + c.defaultSub + ")"
		}
		output += nl + prefix + tab + "subcommands:" + defcmd + nl
	}
	for _, sub := range c.subcmds {
		output += sub.usage(lvl+1) + nl
	}

	return output
}

func (p parsedcmd) run() { // TODO: return error from run() and fn()?
	if p.fn != nil {
		p.fn(&p)
	}
}

func (p parsedcmd) leaf() *parsedcmd {
	if p.sub == nil {
		return &p
	}
	return p.sub.leaf()
}

func (p parsedcmd) usage(lvl int) string {
	if p.usagefn == nil {
		return ""
	}
	return p.usagefn(lvl)
}

// String rebuilds parsedcmd to the original (space trimmed)
// string from which it was parsed.
func (p parsedcmd) String() string {
	if p.sub == nil {
		return p.cmd + " " + strings.Join(p.args, " ")
	}
	return p.cmd + " " + p.sub.String()
}
