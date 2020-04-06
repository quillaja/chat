package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	meProfile := flag.String("profile", "", "profile")
	contactsFile := flag.String("contacts", "", "contacts")
	flag.Parse()

	me, err := ReadProfile(*meProfile)
	if err != nil {
		log.Println(err)
	}

	contacts, err := ReadContacts(*contactsFile)
	if err != nil {
		log.Println(err)
	}

	// output stuff
	output := Color(os.Stdout, Green)
	null, _ := os.Open(os.DevNull)
	defer null.Close()
	enableLog := func(on bool) {
		if on {
			log.SetOutput(Color(os.Stderr, BrightBlack)) // NOTE: color will break in windows terminals
		} else {
			log.SetOutput(null)
		}
	}
	log.SetPrefix("  ")
	enableLog(true)

	ctx, cancel := context.WithCancel(context.Background())

	engine, err := NewChatEngine(me, contacts)
	if err != nil {
		log.Fatalln(err)
	}
	engine.Start(ctx)

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt, os.Kill, syscall.SIGTERM)

	// process input
	cmds := makeCommands()
	cmdQueue := make(chan parsedcmd)
	go console(cmds, cmdQueue)

	for done := false; !done; {
		select {
		case <-sig:
			done = true
			cancel()

		case cmd := <-cmdQueue:

			if cmd.err != nil {
				log.Printf("Error: %s\n", cmd.err)
				continue
			}

			switch cmd.cmd {
			case "help":
				fmt.Fprintln(output, cmds.help()) // uses commanddefs

			case "exit":
				done = true
				cancel()

			case "ip":
				ip, err := GetIP()
				if err != nil {
					log.Println(err)
					continue
				}
				fmt.Fprintf(output, "external IP address:\t%s\nlistening on port:\t%s\n", ip, engine.Me.Port)

			case "me":
				switch cmd = *cmd.leaf(); cmd.cmd {
				case "show":
					fmt.Fprintf(output, "I am \"%s\"\n", engine.Me)

				case "edit":
					p, err := ParseProfile(cmd.args[0])
					if err != nil {
						log.Println(err)
						continue
					}

					err = WriteProfile(p, *meProfile)
					if err != nil {
						log.Println(err)
						continue
					}
					engine.Me = p
				}

			case "contacts":

				switch cmd = *cmd.leaf(); cmd.cmd {
				case "list":
					for i, c := range engine.Contacts {
						if c != nil {
							fmt.Fprintf(output, "%d\t%s\n", i, c)
						}
					}

				case "add":
					var p *Profile
					arg := cmd.args[0]
					n, err := strconv.Atoi(arg)
					if err == nil {
						if sess, ok := engine.GetSession(n); ok {
							p = sess.Other
							if p == nil {
								log.Printf("session %d had a nil Other", n)
								continue
							}
						} else {
							log.Printf("%d not found\n", n)
							continue
						}

					} else {
						p, err = ParseProfile(arg)
						if err != nil {
							log.Println(err)
							continue
						}
					}

					// overwrite contact if existing Equal() one found
					// TODO: do i really want to overwrite? what about having 2
					// contacts with different names but the same address?
					// i guess the question boils down to the definition of Profile
					if index := engine.FindContact(p); index >= 0 {
						old := engine.Contacts[index]
						engine.Contacts[index] = p
						log.Printf("overwrote #%d '%s' with '%s'\n", index, old, p)
					} else {
						engine.AddContact(p)
						log.Printf("added %s\n", p)
					}

					err = WriteContacts(engine.Contacts, *contactsFile)
					if err != nil {
						log.Println(err)
						log.Println("did not save changes to disk")
					}

				case "delete":
					arg := cmd.args[0]
					n, err := strconv.Atoi(arg)
					if err != nil {
						log.Println(err)
						continue
					}

					removed := engine.Contacts[n]
					if engine.RemoveContact(n) {
						log.Printf("deleted %s\n", removed)

						err = WriteContacts(engine.Contacts, *contactsFile)
						if err != nil {
							log.Println(err)
							log.Println("did not save changes to disk")
						}
					} else {
						log.Printf("%d not found\n", n)
					}
				}

			case "requests":
				switch cmd = *cmd.leaf(); cmd.cmd {
				case "list":
					for i, r := range engine.Requests {
						if r != nil {
							fmt.Fprintf(output, "%d\t%s at %s (%s ago)\n", i,
								r.Profile,
								r.Time().Format(time.Kitchen),
								time.Since(r.Time()))
						}
					}

				case "accept":
					arg := cmd.args[0]
					n, err := strconv.Atoi(arg)
					if err != nil {
						log.Println(err)
						continue
					}
					if _, ok := engine.GetRequest(n); !ok {
						log.Printf("%d not found\n", n)
						continue
					}

					err = engine.AcceptRequest(engine.Requests[n])
					if err != nil {
						log.Println(err)
						continue
					}
					log.Println("request accepted")

				case "reject":
					arg := cmd.args[0]
					n, err := strconv.Atoi(arg)
					if err != nil {
						log.Println(err)
						continue
					}

					if engine.RemoveRequest(n) {
						log.Println("removed request")
					} else {
						log.Printf("%d not found\n", n)
					}
				}

			case "sessions":
				switch cmd = *cmd.leaf(); cmd.cmd {
				case "list":
					for i, s := range engine.Sessions {
						if s != nil {
							fmt.Fprintf(output, "%d\t%s\n", i, s)
						}
					}

				case "start":
					var p *Profile
					arg := cmd.args[0]
					n, err := strconv.Atoi(arg)
					if err == nil {
						if p, _ = engine.GetContact(n); p == nil {
							log.Printf("%d not found\n", n)
							continue
						}
					} else {
						p, err = ParseProfile(arg)
						if err != nil {
							log.Println(err)
							continue
						}
						if i := engine.FindContact(p); i >= 0 {
							p = engine.Contacts[i] // use profile from contacts if available
						}

					}

					err = engine.SendRequest(p)
					if err != nil {
						log.Println(err)
						continue
					}
					log.Println("request sent")

				case "drop":
					arg := cmd.args[0]
					n, err := strconv.Atoi(arg)
					if err != nil {
						log.Println(err)
						continue
					}

					if engine.RemoveSession(n) {
						log.Println("dropped session")
					} else {
						log.Printf("%d not found\n", n)
					}
				}

			case "msg":
				n, err := strconv.Atoi(cmd.args[0])
				if err != nil {
					log.Println(err)
					continue
				}
				if _, ok := engine.GetSession(n); !ok {
					log.Printf("%d not found\n", n)
					continue
				}

				err = engine.Sessions[n].SendText(cmd.args[1])
				if err != nil {
					log.Println(err)
					continue
				}
				log.Println("sent")

			case "show":
				n, err := strconv.Atoi(cmd.args[0])
				if err != nil {
					log.Println(err)
					continue
				}
				if s, ok := engine.GetSession(n); !ok {
					log.Printf("%d not found\n", n)
					continue
				} else {
					const num = 5
					start := len(s.Msgs) - num
					if start < 0 {
						start = 0
					} // clamp
					show := s.Msgs[start:]
					for i, t := range show {
						fmt.Fprintf(output, "%d %s\t| %s > %s\n", i,
							t.From().Name,
							t.TimeStamp.Time().Format(time.Kitchen),
							t.Message)
					}
				}
			}
		}
	}

	time.Sleep(500 * time.Millisecond)
	log.Println("exiting program")
}

func console(cmds commanddefs, cmdQueue chan parsedcmd) {
	scan := bufio.NewScanner(os.Stdin)
	for scan.Scan() {
		line := scan.Text()
		pcmd := cmds.parse(line)
		cmdQueue <- pcmd
	}
}

// Color constants.
const (
	wrapper       = "\x1B[%sm"
	reset         = "\x1B[0m"
	Black         = "30"
	Red           = "31"
	Green         = "32"
	Yellow        = "33"
	Blue          = "34"
	Magenta       = "35"
	Cyan          = "36"
	White         = "37"
	BrightBlack   = "90"
	BrightRed     = "91"
	BrightGreen   = "92"
	BrightYellow  = "93"
	BrightBlue    = "94"
	BrightMagenta = "95"
	BrightCyan    = "96"
	BrightWhite   = "97"
)

// Color wraps a writer so subsequent writes are surrounded with
// ANSI escape codes to color console output.
func Color(w io.Writer, color string) io.Writer {
	return scw{
		writer: w,
		color:  fmt.Sprintf(wrapper, color),
	}
}

type scw struct {
	writer io.Writer
	color  string
}

func (w scw) Write(b []byte) (n int, err error) {
	return w.writer.Write([]byte(w.color + string(b) + reset))
}

// GetIP gets the client's external IP address using an external webservice.
func GetIP() (ip string, err error) {
	// uses just amazon, but could use multiple/alt
	resp, err := http.Get("http://checkip.amazonaws.com/")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_, err = fmt.Fscan(resp.Body, &ip)
	return
}

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
		output += c.usage(0) + "\n"
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
	prefix := strings.Repeat(tab, lvl)

	var args []string
	for _, def := range c.args {
		args = append(args, def.name)
	}
	argstring := strings.Join(args, "|")

	output := prefix + c.cmd + tab + argstring + nl
	output += prefix + tab + c.helptext
	if len(c.subcmds) > 0 {
		output += nl + nl + prefix + tab + "subcommands:"
		if c.defaultSub != "" {
			output += " (defaults to " + c.defaultSub + ")"
		}
	}
	for _, sub := range c.subcmds {
		output += nl + sub.usage(lvl+1) + nl
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

// makeCommands defines the REPL commands used in the program.
func makeCommands() commanddefs {
	return commanddefs{
		"help": {
			cmd:      "help",
			helptext: "show information about commands",
		},

		"exit": {
			cmd:      "exit",
			helptext: "exit the chat client",
		},

		"ip": {
			cmd:      "ip",
			helptext: "display your current external IP and port chat client is using",
		},

		"me": {
			cmd:        "me",
			helptext:   "view and change user profile",
			defaultSub: "show",
			subcmds: commanddefs{
				"show": {
					cmd:      "show",
					helptext: "display user information",
				},
				"edit": {
					cmd:      "edit",
					helptext: "modify user information",
					args: []argdef{
						{"PROFILE", re(profile)},
					},
				},
			},
		},

		"contacts": {
			cmd:        "contacts",
			helptext:   "manage contacts",
			defaultSub: "list",
			subcmds: commanddefs{
				"list": {
					cmd:        "list",
					helptext:   "list all contacts",
					defaultSub: "all",
				},
				"add": {
					cmd:      "add",
					helptext: "add a new contact from an existing session or profile",
					args: []argdef{
						{"PROFILE", re(profile)},
						{"SESSION_NUMBER", re(integer)},
					},
				},
				"delete": {
					cmd:      "delete",
					helptext: "delete a contacts",
					args: []argdef{
						{"CONTACT_NUMBER", re(integer)},
					},
				},
			},
		},

		"requests": {
			cmd:        "requests",
			helptext:   "manage requests for chat",
			defaultSub: "list",
			subcmds: commanddefs{
				"list": {
					cmd:      "list",
					helptext: "display waiting requests",
				},
				"accept": {
					cmd:      "accept",
					helptext: "accept chat request and begin a session",
					args: []argdef{
						{"REQUEST_NUMBER", re(integer)},
					},
				},
				"reject": {
					cmd:      "reject",
					helptext: "refuse a chat request",
					args: []argdef{
						{"REQUEST_NUMBER", re(integer)},
					},
				},
			},
		},

		"sessions": {
			cmd:        "sessions",
			helptext:   "manage chat sessions",
			defaultSub: "list",
			subcmds: commanddefs{
				"list": {
					cmd:      "list",
					helptext: "display all pending and active sessions",
				},
				"start": {
					cmd:      "start",
					helptext: "ping another user to a session",
					args: []argdef{
						{"CONTACT_NUMBER", re(integer)},
						{"PROFILE", re(profile)},
					},
				},
				"drop": {
					cmd:      "drop",
					helptext: "end a session",
					args: []argdef{
						{"SESSION_NUMBER", re(integer)},
					},
				},
			},
		},

		"msg": {
			cmd:      "msg",
			helptext: "sends a message",
			args: []argdef{
				{"SESSION_NUMBER MESSAGE", re(integer, rest)},
			},
		},

		"show": {
			cmd:      "show",
			helptext: "show last few messages for a particular session",
			args: []argdef{
				{"SESSION_NUMBER", re(integer)},
			},
		},
	}
}
