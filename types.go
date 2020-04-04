package main

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"
)

/*
Request is sent with key = []byte{}
Response is sent with key = []byte{}, but SharedKey/KeySignature is encrypted and signed by RSA public/private keys
Text is encrypted and signed by AES256 shared key
*/

// ZeroKey is a 'garbage' AES256 key used when pseudo-encrypting Messages
// encoding Request or Response data, when a proper shared key is being
// negotiated. However, all Messages must go through the same processing,
// and even using a non-secure idiotic key like this provides some tiny
// level of obfuscation to transmitted data.
var ZeroKey = make([]byte, 32)

// TimeStamp is unix time (seconds since unix epoc).
type TimeStamp int64

// Now makes time stamp for current time.
func Now() TimeStamp { return TimeStamp(time.Now().Unix()) }

// Time converts time stamp into a time.Time.
func (ts TimeStamp) Time() time.Time { return time.Unix(int64(ts), 0) }

// Profile contains user identification and connection data.
type Profile struct {
	Name    string // name. may contain spaces.
	Address string // example ipv4 "61.2.73.242" or ipv6 "[::1]" or dns "mytld.com"
	Port    string // port without : (colon)
}

// Request is sent to another party when wishing to begin a chat session.
// The initiating client must provide a RSA public key so that the receiving
// client can asymetrically encrypt a proposed shared key in the subsequent Response.
type Request struct {
	Profile   *Profile
	PublicKey *rsa.PublicKey
	TimeStamp // unix time in seconds
}

// Response is sent to another party when a Request is "accepted".
// It contains a proposed shared key (aka secret key) to be used to symetrically
// encrypt subsequent messages (such as Text).
type Response struct {
	Request      *Request // info (ie public key) of client sending Response
	SharedKey    []byte   // RSA encrypted proposed shared key (32 bytes) for this session
	KeySignature []byte   // RSA signature for SharedKey
}

// Text is used to transmit human messages.
type Text struct {
	Message string // ideal max len 1024 bytes
	TimeStamp
	author *Profile // not encoded for transmission
}

//
// Profile stuff
//

// ReadProfile in JSON format from filename.
func ReadProfile(filename string) (*Profile, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	p := &Profile{}
	err = json.Unmarshal(data, p)
	return p, err
}

// WriteProfile in JSON format to filename.
func WriteProfile(p *Profile, filename string) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, data, 0644)
}

// ReadContacts in JSON format from filename.
func ReadContacts(filename string) (contacts []*Profile, err error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}

	err = json.Unmarshal(data, &contacts)
	return
}

// WriteContacts in JSON format to filename.
func WriteContacts(contacts []*Profile, filename string) error {
	data, err := json.MarshalIndent(contacts, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, data, 0644)
}

// ParseProfile parses a string in the form <Name>@<Address>:<Port>
// to a Profile.
func ParseProfile(raw string) (*Profile, error) {
	p := &Profile{}

	parts := strings.SplitN(raw, "@", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("no name")
	}
	p.Name = parts[0]

	parts = strings.Split(parts[1], ":")
	if len(parts) < 2 {
		return nil, fmt.Errorf("no port")
	}
	p.Port = parts[len(parts)-1:][0] // back string. allows ipv6 addresses
	parts = parts[:len(parts)-1]     // front rest

	if len(parts) < 1 {
		return nil, fmt.Errorf("no address")
	}
	p.Address = strings.Join(parts, ":")

	return p, nil
}

// FullAddress gets the profile's Address + Port.
func (p *Profile) FullAddress() string { return p.Address + ":" + p.Port }

//String representation of the profile.
func (p *Profile) String() string { return p.Name + "@" + p.FullAddress() }

//Equal compares profiles based on FullAddress()
func (p *Profile) Equal(o *Profile) bool { return o != nil && p.FullAddress() == o.FullAddress() }

//
// Request stuff
//

// PrepareRequest performs some of the routine tasks involved in building a
// Request, namely generating an RSA key pair.
func PrepareRequest(p *Profile) (*Request, *rsa.PrivateKey, error) {
	if p == nil {
		return nil, nil, fmt.Errorf("nil Profile")
	}

	privateKey, err := GenerateRSAKeyPair()
	if err != nil {
		return nil, nil, err
	}

	r := &Request{
		Profile:   p,
		TimeStamp: Now(),
		PublicKey: &privateKey.PublicKey,
	}
	return r, privateKey, nil
}

// Equal compares one request to another.
func (r *Request) Equal(o *Request) bool { return o != nil && *r == *o }

//
// Text
//

// From gets the profile of the Text writer.
func (t *Text) From() *Profile { return t.author }
