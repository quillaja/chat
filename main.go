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
	cmd := make(chan []string)
	go console(cmd)

	for done := false; !done; {
		select {
		case <-sig:
			done = true
			cancel()

		case parts := <-cmd:
			front, rest := parts[0], parts[1] // rest should contain at least empty string

			switch front {
			case ".help":
				// for _, c := range cmds {
				// 	fmt.Println(c.usage(0) + "\n")
				// }
				fmt.Println(cmds.help())

			case ".exit":
				done = true
				cancel()

			case ".ip":
				ip, err := GetIP()
				if err != nil {
					log.Println(err)
					continue
				}
				fmt.Fprintf(output, "external IP address:\t%s\nlistening on port:\t%s\n", ip, engine.Me.Port)

			case ".me":
				fmt.Fprintf(output, "I am \"%s\"\n", engine.Me)

			case ".me-new":
				if rest == "" {
					log.Println(".me-new [<name>@<address>:<port>]")
					continue
				}
				p, err := ParseProfile(rest)
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

			case ".requests":
				for i, r := range engine.Requests {
					if r != nil {
						fmt.Fprintf(output, "%d\t%s at %s (%s ago)\n", i,
							r.Profile,
							r.Time().Format(time.Kitchen),
							time.Since(r.Time()))
					}
				}

			case ".sessions":
				for i, s := range engine.Sessions {
					if s != nil {
						fmt.Fprintf(output, "%d\t%s\n", i, s)
					}
				}

			case ".contacts":
				for i, c := range engine.Contacts {
					if c != nil {
						fmt.Fprintf(output, "%d\t%s\n", i, c)
					}
				}

			case ".add-contact":
				if rest == "" {
					log.Println(".add-contact [session number] OR [<name>@<address>:<port>]")
					continue
				}
				var p *Profile
				arg := strings.SplitN(rest, " ", 2)[0]
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
					p, err = ParseProfile(rest)
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

			case ".del-contact":
				if rest == "" {
					log.Println(".del-contact [contact number]")
					continue
				}
				arg := strings.SplitN(rest, " ", 2)[0]
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

			case ".ping": // .ping [contact number]
				if rest == "" {
					log.Println(".ping [contact number] OR [<name>@<address>:<port>]")
					continue
				}
				var p *Profile
				arg := strings.SplitN(rest, " ", 2)[0]
				n, err := strconv.Atoi(arg)
				if err == nil {
					if p, _ = engine.GetContact(n); p == nil {
						log.Printf("%d not found\n", n)
						continue
					}
				} else {
					p, err = ParseProfile(rest)
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

			case ".drop": // .drop [session number]
				if rest == "" {
					log.Println(".drop [session number]")
					continue
				}
				arg := strings.SplitN(rest, " ", 2)[0]
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

			case ".accept": // .accept [request number]
				if rest == "" {
					log.Println(".accept [request number]")
					continue
				}
				arg := strings.SplitN(rest, " ", 2)[0]
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

			case ".reject": // .reject [request number]
				if rest == "" {
					log.Println(".reject [request number]")
					continue
				}
				arg := strings.SplitN(rest, " ", 2)[0]
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

			case ".msg": // .msg [session number] [text]
				if rest == "" {
					log.Println(".msg [session number] [text]")
					continue
				}
				parts = strings.SplitN(rest, " ", 2)
				n, err := strconv.Atoi(parts[0])
				if err != nil {
					log.Println(err)
					continue
				}
				if _, ok := engine.GetSession(n); !ok {
					log.Printf("%d not found\n", n)
					continue
				}

				if len(parts) < 2 {
					log.Println("no message content")
					continue
				}

				err = engine.Sessions[n].SendText(parts[1])
				if err != nil {
					log.Println(err)
					continue
				}
				log.Println("sent")

			case ".show": // .show [session number]
				if rest == "" {
					log.Println(".show [session number]")
					continue
				}
				parts = strings.SplitN(rest, " ", 2)
				n, err := strconv.Atoi(parts[0])
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

func console(cmdQueue chan []string) {
	scan := bufio.NewScanner(os.Stdin)
	for scan.Scan() {
		line := scan.Text()
		front, back := split(line)
		cmdQueue <- []string{front, back}

		// log.Printf("%q, %q\n", front, back)
		p := cmds.parse(line)
		// if p.err == nil {
		// log.Println(c)
		// p := c.complete(back)
		// log.Printf("%+v err: %s\n", p, p.err)
		for sub := &p; sub != nil; sub = sub.sub {
			log.Printf("%+v err: %v\n", sub, sub.err)
			// if sub.err != nil {
			// 	fmt.Println(c.usage(0))
			// }
		}
		// }
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
	cmd        string   // cmd such as "contacts" or "msg"
	args       []argdef // list of re to try in order to parse parameters
	subcmds    commanddefs
	defaultSub string // name of default subcommand
	helptext   string
}

type argdef struct {
	name string
	re   *regexp.Regexp // probably have to change this back to string. 'weirdness' without ^ and $
}

type parsedcmd struct {
	cmd  string
	args []string
	sub  *parsedcmd
	err  error
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
	log.Println(c.cmd, "|", rest)

	if len(c.subcmds) > 0 {

		// front, back := split(rest)
		// sub, ok := c.subcmds[front]
		// if !ok {
		// 	sub, ok = c.subcmds[c.defaultSub]
		// 	if !ok {
		// 		p.err = fmt.Errorf("expected subcommand") // no default subcommand
		// 		return
		// 	}
		// 	back = rest // put stripped arg back
		// }
		// s := sub.complete(back)
		// p.sub = &s
		s := c.subcmds.parse(rest)
		if _, ok := c.subcmds[s.cmd]; s.err != nil && !ok { // equal to testing error for 'not a command'
			s = c.subcmds.parse(c.defaultSub + " " + rest)
			// if s.err != nil {
			// 	p.err = s.err
			// 	return
			// }
		}
		p.sub = &s
		p.err = s.err

	} else {

		log.Println("processing args:", rest)
		for _, def := range c.args {
			// re := regexp.MustCompile("^" + def + "$")
			// re := regexp.MustCompile(def)
			// matches := re(defs...).FindStringSubmatch(rest)
			matches := def.re.FindStringSubmatch(rest)
			if len(matches) > 0 {
				p.args = matches[1:]
				break
			}
		}
		if len(c.args) > 0 && len(p.args) == 0 {
			if rest == "" {
				p.err = fmt.Errorf("expected arguments") // couldn't match any arg regexps
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
		output += nl + prefix + sub.usage(lvl+1) + nl
	}
	return output
}

var cmds = commanddefs{
	"msg": {
		cmd:      "msg",
		helptext: "sends a message",
		args: []argdef{
			{"SESSION_NUMBER MESSAGE", re(integer, rest)},
		},
	},

	"contacts": {
		cmd:        "contacts",
		helptext:   "manage contacts",
		defaultSub: "show",
		subcmds: commanddefs{
			"show": {
				cmd:        "show",
				helptext:   "list all contacts",
				defaultSub: "all",
				subcmds: commanddefs{
					"all": {
						cmd:      "all",
						helptext: "does it all",
						args: []argdef{
							{"CONTACT_NUMBER", re(integer)},
						},
					},
				},
			},
			"add": {
				cmd:      "add",
				helptext: "add a new contact",
				args: []argdef{
					{"PROFILE", re(profile)},
					{"SESSION_NUMBER", re(integer)}},
			},
		},
	},
}
