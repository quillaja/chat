package main

import (
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
//var sharedKey = []byte{150, 2, 108, 241, 134, 223, 208, 143, 89, 132, 179, 220, 37, 183, 148, 53, 157, 221, 0, 128, 40, 88, 89, 132, 212, 252, 185, 176, 219, 156, 217, 109}

func main() {
	meProfile := flag.String("profile", "", "profile")
	// port := flag.String("port", "", "listening port")
	toProfile := flag.String("to", "", "send to profile")
	flag.Parse()

	me, err := ReadProfile(*meProfile)
	if err != nil {
		log.Fatalln(err)
	}

	log.Printf("I am %s\n", me)

	sessions := make([]*Session, 0)

	queue := make(chan *Message, 8)
	go Listener(context.Background(), me.Port, queue)

	// send request if toProfile is given
	if *toProfile != "" {
		go func() {
			to, err := ReadProfile(*toProfile)
			if err != nil {
				log.Fatalln(err)
			}

			time.Sleep(1 * time.Second)
			log.Printf("sending request to %s\n", to)

			sess, req, err := InitiateSession(me, to)
			if err != nil {
				log.Fatalln(err)
			}

			err = sess.SendRequest(req)
			if err != nil {
				log.Fatalln(err)
			}
			sessions = append(sessions, sess)
			//sessions[sess.ID()] = sess

			time.Sleep(5 * time.Second)
			err = sess.SendText("hello world")
			if err != nil {
				log.Fatalln(err)
			}
		}()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, os.Kill, syscall.SIGTERM)
	var done bool
	for !done {
		select {
		case <-sig:
			done = true
		case m := <-queue:
			//for m := range queue {

			switch m.Type {
			case PayloadRequest:
				request, err := m.GetRequest()
				if err != nil {
					log.Println(err)
					continue
				}

				var confirm string
				fmt.Printf("accept request from %s? [y/n] ", request.Profile)
				fmt.Scan(&confirm)
				if strings.ToLower(confirm) != "y" {
					log.Println("denied")
					continue
				}
				log.Println("accepted")

				// after accepting a request.
				// 1. begin new (active) session
				// 2. send response
				// 3. add session to manager
				sess, resp, err := BeginSession(me, request)
				if err != nil {
					log.Println(err)
					continue
				}

				err = sess.SendResponse(resp)
				if err != nil {
					log.Println(err)
					continue
				}

				sessions = append(sessions, sess)
				//sessions[sess.ID()] = sess
				log.Printf("began session with %s\n", sess.Other)

			case PayloadResponse:
				resp, err := m.GetResponse()
				if err != nil {
					log.Println(err)
					continue
				}

				// when get response:
				// 1. find Pending session whose PrivKey can decrypt the shared key. (check sig using other pub key)
				// 2. "upgrade" session to Active. fill in SharedKey and OtherPubKey
				var sess *Session
				for _, s := range sessions {
					err := s.Upgrade(resp)
					if err == nil {
						sess = s // found correct session
						break
					}
				}

				if sess != nil {
					log.Printf("began session with %s\n", sess.Other)
				} else {
					log.Printf("no session found for Response from %s\n", resp.Profile)
				}

			case PayloadText:

				// only way to match a Message containing Text to a Session is to
				// try decrypting the message with each session shared key until
				// one works... =/
				var sess *Session
				var text Text
				for _, s := range sessions {
					if s.Status != Active {
						continue
					}

					t, err := m.GetText(s.SharedKey)
					if err == nil {
						sess = s
						text = t
						break
					}
				}

				if sess != nil {
					fmt.Printf(" %s | %s > %s\n", sess.Other, text.Time().Format(time.Kitchen), text.Message)
				} else {
					log.Println("got non-sessioned messaged")
				}
			}
			//}
		}
	}

	for _, s := range sessions {
		fmt.Printf("%+v\n", *s)
	}
}
