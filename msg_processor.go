package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

func (app *Application) MessageProcessor(ctx context.Context) {
	var done bool
	for !done {
		select {
		case <-ctx.Done():
			done = true

		case m := <-app.queue:

			switch m.Type {
			case PayloadRequest:
				request, err := m.GetRequest()
				if err != nil {
					log.Println(err)
					continue
				}

				app.requests = append(app.requests, request)

				// var confirm string
				// fmt.Printf("accept request from %s? [y/n] ", request.Profile)
				// fmt.Scan(&confirm)
				// if strings.ToLower(confirm) != "y" {
				// 	log.Println("denied")
				// 	continue
				// }
				// log.Println("accepted")

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
				for _, s := range app.Sessions {
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
				for _, s := range app.Sessions {
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
					sess.ExtendExpiration()
					fmt.Printf(" %s | %s > %s\n", sess.Other, text.Time().Format(time.Kitchen), text.Message)
				} else {
					log.Println("got non-sessioned messaged")
				}
			}
		}
	}

	log.Println("exiting message processor")
}
