package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"context"
)

// http://checkip.amazonaws.com/

func main() {
	meProfile := flag.String("profile", "", "profile")
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

			case ".ip":
				ip, err := GetIP()
				if err != nil {
					log.Println(err)
					continue
				}
				fmt.Printf("external IP address:\t%s\nlistening on port:\t%s\n", ip, app.Me.Port)

			case ".me":
				fmt.Printf("I am \"%s\"\n", app.Me)

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
				app.Me = p

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

			case ".contacts":
				for i, c := range app.Contacts {
					fmt.Printf("%d\t%s\n", i, c)
				}

			case ".add-contact":
				if rest == "" {
					log.Println(".add-contact [session number]")
					continue
				}
				arg := strings.SplitN(rest, " ", 2)[0]
				n, err := strconv.Atoi(arg)
				if err != nil {
					log.Println(err)
					continue
				}
				if n < 0 || n >= len(app.Sessions) {
					log.Printf("%d not found\n", n)
					continue
				}

				// find index where p == contacts[index]
				p := app.Sessions[n].Other
				index := -1
				for i := range app.Contacts {
					if p.Equal(app.Contacts[i]) {
						index = i
						break
					}
				}

				newContacts := make([]Profile, len(app.Contacts))
				copy(newContacts, app.Contacts)
				if index >= 0 {
					old := newContacts[index]
					newContacts[index] = p
					log.Printf("overwrote #%d '%s' with '%s'\n", index, old, p)
				} else {
					newContacts = append(newContacts, p)
					log.Printf("added %s\n", p)
				}

				err = WriteContacts(newContacts, *contactsFile)
				if err != nil {
					log.Println(err)
					log.Println("no changes saved")
					continue
				}
				app.Contacts = newContacts

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
				if n < 0 || n >= len(app.Contacts) {
					log.Printf("%d not found\n", n)
					continue
				}

				p := app.Contacts[n]
				newContacts := make([]Profile, len(app.Contacts))
				copy(newContacts, app.Contacts)
				newContacts = append(newContacts[:n], newContacts[n+1:]...)
				log.Printf("deleted %s\n", p)

				err = WriteContacts(newContacts, *contactsFile)
				if err != nil {
					log.Println(err)
					log.Println("no changes saved")
					continue
				}
				app.Contacts = newContacts

			case ".ping": // .ping [contact number]
				if rest == "" {
					log.Println(".ping [contact number]")
					continue
				}
				arg := strings.SplitN(rest, " ", 2)[0]
				n, err := strconv.Atoi(arg)
				if err != nil {
					log.Println(err)
					continue
				}
				if n < 0 || n >= len(app.Contacts) {
					log.Printf("%d not found\n", n)
					continue
				}

				err = app.SendRequest(app.Contacts[n])
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
				if n < 0 || n >= len(app.Sessions) {
					log.Printf("%d not found\n", n)
					continue
				}

				app.Sessions = append(app.Sessions[:n], app.Sessions[n+1:]...)
				log.Println("dropped session")

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
				if n < 0 || n >= len(app.requests) {
					log.Printf("%d not found\n", n)
					continue
				}

				err = app.AcceptRequest(app.requests[n])
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
				if n < 0 || n >= len(app.requests) {
					log.Printf("%d not found\n", n)
					continue
				}

				app.requests = append(app.requests[:n], app.requests[n+1:]...)
				log.Println("removed request")

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
				if n < 0 || n >= len(app.Sessions) {
					log.Printf("%d not found\n", n)
					continue
				}

				if len(parts) < 2 {
					log.Println("no message content")
					continue
				}

				err = app.Sessions[n].SendText(parts[1])
				if err != nil {
					log.Println(err)
					continue
				}
				log.Println("sent")

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
		// log.Printf("%q\n", parts)
		cmdQueue <- parts
	}
}

func GetIP() (ip string, err error) {
	resp, err := http.Get("http://checkip.amazonaws.com/")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_, err = fmt.Fscan(resp.Body, &ip)
	return
}

type Application struct {
	Me       Profile
	Contacts []Profile
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
		contacts = make([]Profile, 0)
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

	// remove request from waiting list
	for i, r := range app.requests {
		if request == r {
			// a = a[:i+copy(a[i:], a[i+1:])]
			//app.requests = app.requests[:i+copy(app.requests[i:], app.requests[i+i:])] // remove request
			// a = append(a[:i], a[i+1:]...)
			app.requests = append(app.requests[:i], app.requests[i+1:]...)
			break
		}
	}

	// TODO: ?? add/modify contact list with new/updated Profile?
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
