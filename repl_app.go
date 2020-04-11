package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// App is the basic type
type App interface {
	Run() // should block until complete
}

// ReplApp is an App that provides a REPL shell for user interaction.
type ReplApp struct {
	commands      commanddefs
	console       *Console
	engine        *ChatEngine
	output        io.Writer
	meProfileFile string
	contactsFile  string
}

// NewReplApp creates a new App.
// need config for:
// profile/contacts input file
// output & log config
// prompt/console config
func NewReplApp(meProfileFile, contactsFile string, output io.Writer) App {
	ui := new(ReplApp)
	ui.meProfileFile = meProfileFile
	ui.contactsFile = contactsFile
	ui.output = output
	ui.setupCommands()

	// read profile/contacts
	me, err := ReadProfile(meProfileFile)
	if err != nil {
		log.Println(err)
	}

	contacts, err := ReadContacts(contactsFile)
	if err != nil {
		log.Println(err)
	}

	// setup engine
	ui.engine, err = NewChatEngine(me, contacts)
	if err != nil {
		log.Fatalln(err)
	}

	// setup console
	ui.console = NewConsole(os.Stdin)
	ui.console.Format = func() string { return time.Now().Format("3:04:05 PM") + " > " }
	// ui.console.Format = func() string { return ui.engine.Me.Name + " > " }

	return ui
}

// Run starts the app. It blocks until the app finishes.
func (ui *ReplApp) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // stops engine

	// start chat engine
	ui.engine.Start(ctx)
	// start repl console
	ui.console.Run(ctx)

	ui.loop() // blocks until "quit"
}

// setupCommands defines the REPL commands used in the program.
func (ui *ReplApp) setupCommands() {
	ui.commands = commanddefs{
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

// loop performs the read and loop (RL) of the REPL. It also
// responds to SIGINT and SIGTERM to close the app.
func (ui *ReplApp) loop() {
	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	for {
		// get first input from sig, console, or bot
		var line string
		select {
		case <-sig:
			return

		case line = <-ui.console.Read():
			if ui.evalLine(line) {
				return
			}

		}
	}
}

// evalLine performs the eval and print (EP) of the REPL.
func (ui *ReplApp) evalLine(line string) (quit bool) {
	// these vars are used in many places below
	engine := ui.engine
	cmds := ui.commands
	output := ui.output
	meProfileFile := ui.meProfileFile
	contactsFile := ui.contactsFile

	// parse raw line into command struct
	cmd := cmds.parse(line)
	if cmd.err != nil {
		if cmd.cmd != "" { // ignore blank lines
			log.Printf("Error: %s\n", cmd.err)
		}
		return
	}

	// process specific command
	// each block could really be in it's own function
	// or a function in the command definitions
	switch cmd.cmd {
	case "exit":
		return true

	case "help":
		fmt.Fprintln(output, cmds.help()) // uses commanddefs

	case "ip":
		fmt.Fprintln(output, "getting external ip...")
		ip, err := GetIP()
		if err != nil {
			log.Println(err)
			return
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
				return
			}

			err = WriteProfile(p, meProfileFile)
			if err != nil {
				log.Println(err)
				return
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
						return
					}
				} else {
					log.Printf("%d not found\n", n)
					return
				}

			} else {
				p, err = ParseProfile(arg)
				if err != nil {
					log.Println(err)
					return
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

			err = WriteContacts(engine.Contacts, contactsFile)
			if err != nil {
				log.Println(err)
				log.Println("did not save changes to disk")
			}

		case "delete":
			arg := cmd.args[0]
			n, err := strconv.Atoi(arg)
			if err != nil {
				log.Println(err)
				return
			}

			removed := engine.Contacts[n]
			if engine.RemoveContact(n) {
				log.Printf("deleted %s\n", removed)

				err = WriteContacts(engine.Contacts, contactsFile)
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
				return
			}
			if _, ok := engine.GetRequest(n); !ok {
				log.Printf("%d not found\n", n)
				return
			}

			err = engine.AcceptRequest(engine.Requests[n])
			if err != nil {
				log.Println(err)
				return
			}
			log.Println("request accepted")

		case "reject":
			arg := cmd.args[0]
			n, err := strconv.Atoi(arg)
			if err != nil {
				log.Println(err)
				return
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
					return
				}
			} else {
				p, err = ParseProfile(arg)
				if err != nil {
					log.Println(err)
					return
				}
				if i := engine.FindContact(p); i >= 0 {
					p = engine.Contacts[i] // use profile from contacts if available
				}

			}

			err = engine.SendRequest(p)
			if err != nil {
				log.Println(err)
				return
			}
			log.Println("request sent")

		case "drop":
			arg := cmd.args[0]
			n, err := strconv.Atoi(arg)
			if err != nil {
				log.Println(err)
				return
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
			return
		}
		if _, ok := engine.GetSession(n); !ok {
			log.Printf("%d not found\n", n)
			return
		}

		err = engine.Sessions[n].SendText(cmd.args[1])
		if err != nil {
			log.Println(err)
			return
		}
		log.Println("sent")

	case "show":
		n, err := strconv.Atoi(cmd.args[0])
		if err != nil {
			log.Println(err)
			return
		}
		s, ok := engine.GetSession(n)
		if !ok {
			log.Printf("%d not found\n", n)
			return
		}

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

	return
}
