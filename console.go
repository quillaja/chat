package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
)

// Console abstracts the line-by-line reading of text, using an optional prompt.
type Console struct {
	Format     func() string
	prompt     string
	showPrompt bool
	reader     io.Reader
	lines      chan string
	scan       *bufio.Scanner
}

// NewConsole makes a new Console.
func NewConsole(r io.Reader) *Console {
	c := &Console{
		showPrompt: true,
		reader:     r,
		lines:      make(chan string),
		scan:       bufio.NewScanner(r),
	}
	return c
}

// Run begins the read loop on Console's io.Reader.
//
// ctx is not actually used.
func (c *Console) Run(ctx context.Context) {
	go func() {
		// this gofunc never exits "correctly" since i can't figure
		// out how to "unblock" ReadString()
		for {
			c.scan.Scan()
			line := c.scan.Text()
			err := c.scan.Err()
			// line, err := c.scan.ReadString('\n')
			// line = strings.TrimSuffix(line, "\n") // trim trailing newline
			if err == nil { //&& len(line) > 0 {
				c.lines <- line
			}
			if err != nil {
				log.Println(err)
			}
		}
	}()
}

// EnablePrompt turns on or off the prompt display.
func (c *Console) EnablePrompt(on bool) {
	c.showPrompt = on
}

// Read returns a channel from which a 'line' can be obtained.
func (c *Console) Read() chan string {
	if c.showPrompt && c.Format != nil {
		fmt.Print(c.Format())
	}
	return c.lines
}

// SetPrompt sets the prompt according to Format where s is the "%s" term in Format.
// If Format is an empty string, the prompt is set to s.
// func (c *Console) SetPrompt(s string) {
// 	if c.Format == "" {
// 		c.prompt = s
// 	} else {
// 		c.prompt = fmt.Sprintf(c.Format, s)
// 	}
// }
