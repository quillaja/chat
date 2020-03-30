package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"context"
)

// http://checkip.amazonaws.com/

func main() {
	meProfile := flag.String("profile", "", "profile")
	// toProfile := flag.String("to", "", "send to profile")
	contactsFile := flag.String("contacts", "", "contacts")
	flag.Parse()

	app, err := NewApplication(*meProfile, *contactsFile)
	if err != nil {
		log.Fatalln(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go app.Listener(ctx)
	go app.MessageProcessor(ctx)

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
			case ".exit":
				done = true
				cancel()

			case ".me":
				fmt.Printf("I am %s\n", app.Me)

			case ".contacts":
				for k, c := range app.Contacts {
					fmt.Printf("%s\t%s\n", k, c)
				}

			case ".requests":
				for i, r := range app.requests {
					fmt.Printf("%d\t%s at %s (%s ago)\n", i,
						r.Profile,
						r.Time().Format(time.Kitchen),
						time.Since(r.Time()))
				}

			case ".sessions":
				for i, s := range app.Sessions {
					fmt.Printf("%d\t%s\n", i, s)
				}

			case ".ping": // .ping [contact key]
				if rest == "" {
					log.Println(".ping [contact key]")
					continue
				}
				key := strings.SplitN(rest, " ", 2)[0]
				other, ok := app.Contacts[key]
				if !ok {
					log.Printf("no contact with address %s\n", key)
					continue
				}
				err := app.SendRequest(other)
				if err != nil {
					log.Println(err)
				} else {
					log.Println("request sent")
				}

			case ".drop": // .drop [session number]

			case ".accept": // .accept [request number]
			case ".reject": // .reject [request number]

			case ".msg": // .msg [session number] [text]
				fmt.Println("sending message to", rest)

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
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 1 { // empty string
			continue
		}
		if len(parts) == 1 {
			parts = append(parts, "") //ensure 'rest' always has something
		}
		log.Printf("%q\n", parts)
		cmdQueue <- parts
	}
}

type Application struct {
	Me       Profile
	Contacts map[string]Profile
	Sessions []*Session
	queue    chan *Message
	requests []Request // requests needing approval
}

func NewApplication(profilePath, contactsPath string) (*Application, error) {
	me, err := ReadProfile(profilePath)
	if err != nil {
		return nil, err
	}

	contacts, err := ReadContacts(contactsPath)
	if err != nil {
		log.Println(err)
		contacts = make(map[string]Profile)
	}

	return &Application{
		Me:       me,
		Contacts: contacts,
		Sessions: make([]*Session, 0),
		queue:    make(chan *Message, 8),
		requests: make([]Request, 0),
	}, nil
}

func (app *Application) AcceptRequest(request Request) error {
	// after accepting a request.
	// 1. begin new (active) session
	// 2. send response
	// 3. add session to manager

	sess, resp, err := BeginSession(app.Me, request)
	if err != nil {
		return err
	}

	err = sess.SendResponse(resp)
	if err != nil {
		return err
	}

	app.Sessions = append(app.Sessions, sess)
	log.Printf("began session with %s\n", sess.Other)
	return nil
}

func (app *Application) SendRequest(to Profile) error {
	sess, req, err := InitiateSession(app.Me, to)
	if err != nil {
		return err
	}

	err = sess.SendRequest(req)
	if err != nil {
		return err
	}

	app.Sessions = append(app.Sessions, sess)
	return nil
}
