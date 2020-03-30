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

var ZeroKey = make([]byte, 32)

type TimeStamp int64

func Now() TimeStamp                 { return TimeStamp(time.Now().Unix()) }
func (ts TimeStamp) Time() time.Time { return time.Unix(int64(ts), 0) }

type Profile struct {
	Name    string // name. may contain spaces.
	Address string // example ipv4 "61.2.73.242" or ipv6 "[::1]" or dns "mytld.com"
	Port    string // port without : (colon)
}

type Request struct {
	Profile
	rsa.PublicKey
	TimeStamp // unix time in seconds
}

type Response struct {
	Request
	SharedKey    []byte // RSA encrypted proposed shared key (32 bytes) for this session
	KeySignature []byte // RSA signature for SharedKey
}

type Text struct {
	Message string // ideal max len 1024 bytes
	TimeStamp
}

func PrepareRequest(p Profile) (Request, *rsa.PrivateKey, error) {
	privateKey, err := GenerateRSAKeyPair()
	if err != nil {
		return Request{}, nil, err
	}

	r := Request{
		Profile:   p,
		TimeStamp: Now(),
		PublicKey: privateKey.PublicKey,
	}
	return r, privateKey, nil
}

func ReadProfile(filename string) (p Profile, err error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	err = json.Unmarshal(data, &p)
	return
}

func WriteProfile(p Profile, filename string) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, data, 0644)
}

func ReadContacts(filename string) (contacts []Profile, err error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}

	err = json.Unmarshal(data, &contacts)
	return
}

func WriteContacts(contacts []Profile, filename string) error {
	data, err := json.MarshalIndent(contacts, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, data, 0644)
}

func ParseProfile(raw string) (p Profile, err error) {
	parts := strings.SplitN(raw, "@", 2)
	if len(parts) < 2 {
		err = fmt.Errorf("no name")
		return
	}
	p.Name = parts[0]

	parts = strings.Split(parts[1], ":")
	if len(parts) < 2 {
		err = fmt.Errorf("no port")
		return
	}
	p.Port = parts[len(parts)-1:][0] // back string. allows ipv6 addresses
	parts = parts[:len(parts)-1]     // front rest

	if len(parts) < 1 {
		err = fmt.Errorf("no address")
		return
	}
	p.Address = strings.Join(parts, ":")

	return
}

func (p Profile) FullAddress() string { return p.Address + ":" + p.Port }

func (p Profile) String() string { return p.Name + "@" + p.FullAddress() }

func (p Profile) Equal(o Profile) bool { return p.FullAddress() == o.FullAddress() }
