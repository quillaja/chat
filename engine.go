package main

import (
	"context"
	"log"
)

// ChatEngine incorporates all the nitty gritty of dealing with other chat clients.
// It also simplifys managment of various state by the User Interface and
// provides a mechanism for incoming events to be communicated to the User Interface.
type ChatEngine struct {
	Me       *Profile         // profile in use by this client
	Contacts []*Profile       // a list of known profiles
	Sessions []*Session       // chat sessions of all status
	Requests []*Request       // requests needing approval
	Events   chan EngineEvent // incoming events to signal the UI that something needs done
	queue    chan *Message    // queue of messages between Listener() and MessageProcessor()
}

// EngineEvent communicates engine events to the User Interface.
type EngineEvent struct {
	Data  interface{} // pointer Request, Response, Profile, Session associated with event
	Index int         // index of Data in an associated slice, if applicable
	Type  EventType   // General event type. Specifics determined by Type and type of Data.
}

// EventType simple type of event.
type EventType int

// NewChatEngine initializes a new chat engine.
func NewChatEngine(profilePath, contactsPath string) (*ChatEngine, error) {
	me, err := ReadProfile(profilePath)
	if err != nil {
		// TODO: if no profile, use IP from GetIP() and a default port (eg 5190 (old AIM port))?
		return nil, err
	}

	contacts, err := ReadContacts(contactsPath)
	if err != nil {
		log.Println(err)
		contacts = make([]*Profile, 0)
	}

	return &ChatEngine{
		Me:       me,
		Contacts: contacts,
		Sessions: make([]*Session, 0),
		Requests: make([]*Request, 0),
		Events:   make(chan EngineEvent, 16),
		queue:    make(chan *Message, 16),
	}, nil
}

// Start kicks off sub processes of the engine.
func (eng *ChatEngine) Start(ctx context.Context) {
	go eng.Listener(ctx)
	go eng.MessageProcessor(ctx)
}

// AcceptRequest performs the routine work in responding positively (accepting)
// to a Request to chat. It manages Session state and sends an affirmative
// Response to the other client.
func (eng *ChatEngine) AcceptRequest(request *Request) error {
	// after accepting a request.
	// 1. begin new (active) session
	// 2. send response
	// 3. add session to manager

	sess, resp, err := BeginSession(eng.Me, request)
	if err != nil {
		return err
	}

	err = sess.SendResponse(resp)
	if err != nil {
		return err
	}

	// remove request from waiting list
	eng.RemoveRequest(eng.FindRequest(request))

	// TODO: ?? add/modify contact list with new/updated Profile?
	eng.AddSession(sess)
	log.Printf("began session with %s\n", sess.Other)
	return nil
}

// SendRequest performs the routine work in asking another client to chat.
// This includes Session managmenent and sending a Request to ther other client.
func (eng *ChatEngine) SendRequest(to *Profile) error {
	sess, req, err := InitiateSession(eng.Me, to)
	if err != nil {
		return err
	}

	err = sess.SendRequest(req)
	if err != nil {
		return err
	}

	eng.AddSession(sess)
	return nil
}

//
// Get, Find, Add, Remove series of functions for
// Contacts, Sessions, and Requests.
//

//
// Get
//

// GetContact returns the item at index and a boolean indicating if the
// index was in bounds and contained a non-nil item.
func (eng *ChatEngine) GetContact(index int) (item *Profile, ok bool) {
	if index < 0 || index >= len(eng.Contacts) ||
		eng.Contacts[index] == nil {
		return nil, false
	}
	return eng.Contacts[index], true
}

// GetSession returns the item at index and a boolean indicating if the
// index was in bounds and contained a non-nil item.
func (eng *ChatEngine) GetSession(index int) (item *Session, ok bool) {
	if index < 0 || index >= len(eng.Sessions) ||
		eng.Sessions[index] == nil {
		return nil, false
	}
	return eng.Sessions[index], true
}

// GetRequest returns the item at index and a boolean indicating if the
// index was in bounds and contained a non-nil item.
func (eng *ChatEngine) GetRequest(index int) (item *Request, ok bool) {
	if index < 0 || index >= len(eng.Requests) ||
		eng.Requests[index] == nil {
		return nil, false
	}
	return eng.Requests[index], true
}

//
// Find
//

// FindContact returns index of first contact Equal() to the param, or -1
// if not found.
func (eng *ChatEngine) FindContact(p *Profile) int {
	if p == nil {
		return -1
	}

	for i, o := range eng.Contacts {
		if p == o || p.Equal(o) {
			return i
		}
	}
	return -1
}

// FindSession returns index of first session Equal() to the param, or -1
// if not found.
func (eng *ChatEngine) FindSession(s *Session) int {
	if s == nil {
		return -1
	}

	for i, o := range eng.Sessions {
		if s == o || s.Equal(o) {
			return i
		}
	}
	return -1
}

// FindRequest returns index of first request Equal() to the param, or -1
// if not found.
func (eng *ChatEngine) FindRequest(r *Request) int {
	if r == nil {
		return -1
	}

	for i, o := range eng.Requests {
		if r == o || r.Equal(o) {
			return i
		}
	}
	return -1
}

//
// Add
//

// AddContact adds the contact. Returns index of added item.
func (eng *ChatEngine) AddContact(p *Profile) int {
	if p == nil {
		return -1
	}

	// attempt insert at first nil
	for i := range eng.Contacts {
		if eng.Contacts[i] == nil {
			eng.Contacts[i] = p
			return i
		}
	}

	eng.Contacts = append(eng.Contacts, p)
	return len(eng.Contacts) - 1
}

// AddSession adds the session. Returns index of added item.
func (eng *ChatEngine) AddSession(s *Session) int {
	if s == nil {
		return -1
	}

	// attempt insert at first nil
	for i := range eng.Sessions {
		if eng.Sessions[i] == nil {
			eng.Sessions[i] = s
			return i
		}
	}

	eng.Sessions = append(eng.Sessions, s)
	return len(eng.Sessions) - 1
}

// AddRequest adds the Request. Returns index of added item.
func (eng *ChatEngine) AddRequest(r *Request) int {
	if r == nil {
		return -1
	}

	// attempt insert at first nil
	for i := range eng.Requests {
		if eng.Requests[i] == nil {
			eng.Requests[i] = r
			return i
		}
	}

	eng.Requests = append(eng.Requests, r)
	return len(eng.Requests) - 1
}

//
// Remove
//

// RemoveContact removes the Contact at index.
// Ignores out-of-bound indices. Return value indicates if item
// was successfully removed.
func (eng *ChatEngine) RemoveContact(index int) bool {
	if index < 0 || index >= len(eng.Contacts) {
		return false
	}

	eng.Contacts[index] = nil
	return true
}

// RemoveSession removes the Session at index.
// Ignores out-of-bound indices. Return value indicates if item
// was successfully removed.
func (eng *ChatEngine) RemoveSession(index int) bool {
	if index < 0 || index >= len(eng.Sessions) {
		return false
	}

	eng.Sessions[index] = nil
	return true
}

// RemoveRequest removes the Request at index.
// Ignores out-of-bound indices. Return value indicates if item
// was successfully removed.
func (eng *ChatEngine) RemoveRequest(index int) bool {
	if index < 0 || index >= len(eng.Requests) {
		return false
	}

	eng.Requests[index] = nil
	return true
}
