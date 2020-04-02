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
	"strings"
	"syscall"
	"time"
)

func main() {
	meProfile := flag.String("profile", "", "profile")
	contactsFile := flag.String("contacts", "", "contacts")
	flag.Parse()

	app, err := NewApplication(*meProfile, *contactsFile)
	if err != nil {
		log.Fatalln(err)
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
				fmt.Fprintf(output, "external IP address:\t%s\nlistening on port:\t%s\n", ip, app.Me.Port)

			case ".me":
				fmt.Fprintf(output, "I am \"%s\"\n", app.Me)

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
				for i, r := range app.Requests {
					fmt.Fprintf(output, "%d\t%s at %s (%s ago)\n", i,
						r.Profile,
						r.Time().Format(time.Kitchen),
						time.Since(r.Time()))
				}

			case ".sessions":
				for i, s := range app.Sessions {
					fmt.Fprintf(output, "%d\t%s\n", i, s)
				}

			case ".contacts":
				for i, c := range app.Contacts {
					fmt.Fprintf(output, "%d\t%s\n", i, c)
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
					if n < 0 || n >= len(app.Sessions) {
						log.Printf("%d not found\n", n)
						continue
					}
					p = app.Sessions[n].Other

				} else {
					p, err = ParseProfile(rest)
					if err != nil {
						log.Println(err)
						continue
					}
				}

				// copy and alter copy
				// to allow "revert" if write-to-disk fails
				newContacts := make([]*Profile, len(app.Contacts))
				copy(newContacts, app.Contacts)
				if index := searchProfiles(app.Contacts, p); index >= 0 {
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

				// copy and alter copy
				// to allow "revert" if write-to-disk fails
				newContacts := make([]*Profile, len(app.Contacts))
				copy(newContacts, app.Contacts)
				newContacts = append(newContacts[:n], newContacts[n+1:]...)
				log.Printf("deleted %s\n", app.Contacts[n])

				err = WriteContacts(newContacts, *contactsFile)
				if err != nil {
					log.Println(err)
					log.Println("no changes saved")
					continue
				}
				app.Contacts = newContacts

			case ".ping": // .ping [contact number]
				if rest == "" {
					log.Println(".ping [contact number] OR [<name>@<address>:<port>]")
					continue
				}
				var p *Profile
				arg := strings.SplitN(rest, " ", 2)[0]
				n, err := strconv.Atoi(arg)
				if err == nil {
					if n < 0 || n >= len(app.Contacts) {
						log.Printf("%d not found\n", n)
						continue
					}
					p = app.Contacts[n]
				} else {
					p, err = ParseProfile(rest)
					if err != nil {
						log.Println(err)
						continue
					}
					if i := searchProfiles(app.Contacts, p); i >= 0 {
						p = app.Contacts[i] // use profile from contacts if available
					}
				}

				err = app.SendRequest(p)
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
				if n < 0 || n >= len(app.Requests) {
					log.Printf("%d not found\n", n)
					continue
				}

				err = app.AcceptRequest(app.Requests[n])
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
				if n < 0 || n >= len(app.Requests) {
					log.Printf("%d not found\n", n)
					continue
				}

				app.Requests = append(app.Requests[:n], app.Requests[n+1:]...)
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

// returns first index where list contains a profile Equal() to p, or -1
// if not found.
func searchProfiles(list []*Profile, p *Profile) int {
	for i := range list {
		if p.Equal(list[i]) {
			return i
		}
	}
	return -1
}

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

type Application struct {
	Me       *Profile
	Contacts []*Profile
	Sessions []*Session
	Requests []*Request // requests needing approval
	queue    chan *Message
}

func NewApplication(profilePath, contactsPath string) (*Application, error) {
	me, err := ReadProfile(profilePath)
	if err != nil {
		return nil, err
	}

	contacts, err := ReadContacts(contactsPath)
	if err != nil {
		log.Println(err)
		contacts = make([]*Profile, 0)
	}

	return &Application{
		Me:       me,
		Contacts: contacts,
		Sessions: make([]*Session, 0),
		Requests: make([]*Request, 0),
		queue:    make(chan *Message, 8),
	}, nil
}

func (app *Application) AcceptRequest(request *Request) error {
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
	for i, r := range app.Requests {
		if request == r {
			// a = a[:i+copy(a[i:], a[i+1:])]
			//app.requests = app.requests[:i+copy(app.requests[i:], app.requests[i+i:])] // remove request
			// a = append(a[:i], a[i+1:]...)
			app.Requests = append(app.Requests[:i], app.Requests[i+1:]...)
			break
		}
	}

	// TODO: ?? add/modify contact list with new/updated Profile?
	app.Sessions = append(app.Sessions, sess)
	log.Printf("began session with %s\n", sess.Other)
	return nil
}

func (app *Application) SendRequest(to *Profile) error {
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
