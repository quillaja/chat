package main

import (
	"context"
	"log"
)

// MessageProcessor runs a loop consuming, decoding, and processing
// Messages received from Listener().
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

				eng.AddRequest(request)
				// TODO: event to UI
				log.Printf("got request from %s\n", request.Profile)

			case PayloadResponse:
				resp, err := m.GetResponse()
				if err != nil {
					log.Println(err)
					continue
				}

				// when get response:
				// 1. find Pending session whose PrivKey can decrypt the shared key.
				// 2. "upgrade" session to Active. fill in SharedKey and OtherPubKey
				var sess *Session
				for _, s := range eng.Sessions {
					if s == nil {
						continue
					}
					err := s.Upgrade(resp) // upgrade will only work with correct key
					if err == nil {
						sess = s // found correct session
						// TODO: ?? modify contact list with (potentially) updated Profile?
						break
					}
				}

				if sess != nil {
					// TODO: event to UI
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
					sess.PushIn(text)
					// TODO: event to UI
					log.Printf("new message for session %d\n", sessNumber)
				} else {
					log.Println("got non-sessioned message")
				}
			}
		}
	}

	log.Println("exiting message processor")
}
