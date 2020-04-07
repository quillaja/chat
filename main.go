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
	"strconv"
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
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	// process input
	cmds := makeCommands()

	console := NewConsole(os.Stdin)
	console.Format = "%s > "
	go console.Run()
	botR, botW := io.Pipe()
	bot := NewConsole(botR)
	go bot.Run()

	// go func() {
	// 	console.Prompt(false)
	// 	// time.Sleep(5 * time.Second)
	// 	botW.Write([]byte("me\n"))
	// 	// time.Sleep(5 * time.Second)
	// 	botW.Write([]byte("ip\n"))
	// 	console.Prompt(true)
	// }()
	// time.Sleep(100 * time.Millisecond) // give 'bot' chance to run

	for done := false; !done; {
		var line string
		console.SetPrompt(time.Now().Format("3:04:05 PM"))

		// get first input from sig, console, or bot
		select {
		case <-sig:
			done = true
			cancel()

		case line = <-bot.Read():
		case line = <-console.Read():
		}

		// parse raw line into command struct
		cmd := cmds.parse(line)
		if cmd.err != nil {
			if cmd.cmd != "" { // ignore blank lines
				log.Printf("Error: %s\n", cmd.err)
			}
			continue
		}

		// process specific command
		switch cmd.cmd {
		case "exit":
			done = true
			cancel()

		case "help":
			// fmt.Fprintln(output, cmds.help()) // uses commanddefs

			// this seems to "Work"
			go func() {
				console.EnablePrompt(false)
				botW.Write([]byte("me\n"))
				time.Sleep(5 * time.Second)
				botW.Write([]byte("ip\n"))
				console.EnablePrompt(true)
			}()
			time.Sleep(100 * time.Millisecond) // give 'bot' chance to run

		case "ip":
			fmt.Fprintln(output, "getting external ip...")
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

	time.Sleep(500 * time.Millisecond)
	log.Println("exiting program")
}

// Run begins the read loop on Console's io.Reader. It blocks, so normally
// call this method as a gofunc.
func (c *Console) Run() {
	// go func() {
	// this gofunc never exits "correctly" since i can't figure
	// out how to "unblock" ReadString()
	for {
		c.scan.Scan()
		line := c.scan.Text()
		err := c.scan.Err()
		// line, err := c.scan.ReadString('\n')
		// line = strings.TrimSuffix(line, "\n") // trim trailing newline
		if err == nil { //&& len(line) > 0 {
			c.lines <- line
		}
		if err != nil {
			log.Println(err)
		}
	}
	// }()
}

// EnablePrompt turns on or off the prompt display.
func (c *Console) EnablePrompt(on bool) {
	c.showPrompt = on
}

// Read returns a channel from which a 'line' can be obtained.
func (c *Console) Read() chan string {
	if c.showPrompt {
		fmt.Print(c.prompt)
	}
	return c.lines
}

// SetPrompt sets the prompt according to Format where s is the "%s" term in Format.
// If Format is an empty string, the prompt is set to s.
func (c *Console) SetPrompt(s string) {
	if c.Format == "" {
		c.prompt = s
	} else {
		c.prompt = fmt.Sprintf(c.Format, s)
	}
}

// Console abstracts the line-by-line reading of text, using an optional prompt.
type Console struct {
	Format     string
	prompt     string
	showPrompt bool
	reader     io.Reader
	lines      chan string
	scan       *bufio.Scanner
}

// NewConsole makes a new Console.
func NewConsole(r io.Reader) *Console {
	c := &Console{
		showPrompt: true,
		reader:     r,
		lines:      make(chan string),
		scan:       bufio.NewScanner(r),
	}
	return c
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
