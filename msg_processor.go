package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

func (eng *ChatEngine) MessageProcessor(ctx context.Context) {
	var done bool
	for !done {
		select {
		case <-ctx.Done():
			done = true

		case m := <-eng.queue:

			switch m.Type {
			case PayloadRequest:
				request, err := m.GetRequest()
				if err != nil {
					log.Println(err)
					continue
				}

				eng.Requests = append(eng.Requests, request)
				log.Printf("got request from %s\n", request.Profile)

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
				for _, s := range eng.Sessions {
					err := s.Upgrade(resp) // upgrade will only work with correct key
					if err == nil {
						sess = s // found correct session
						// TODO: ?? modify contact list with (potentially) updated Profile?
						break
					}
				}

				if sess != nil {
					log.Printf("began session with %s\n", sess.Other)
				} else {
					log.Printf("no session found for Response from %s\n", resp.Request.Profile)
				}

			case PayloadText:

				// only way to match a Message containing Text to a Session is to
				// try decrypting the message with each session shared key until
				// one works... =/
				var sess *Session
				var sessNumber int
				var text *Text
				for i, s := range eng.Sessions {
					if s == nil || s.Status != Active {
						continue
					}

					t, err := m.GetText(s.SharedKey)
					if err == nil {
						sess = s
						sessNumber = i
						text = t
						break
					}
				}

				if sess != nil {
					sess.ExtendExpiration()
					// TODO: change to use future 'ui' interface
					fmt.Printf("(%d) %s | %s >\n\t%s\n",
						sessNumber,
						sess.Other,
						text.Time().Format(time.Kitchen),
						text.Message)
				} else {
					log.Println("got non-sessioned message")
				}
			}
		}
	}

	log.Println("exiting message processor")
}
