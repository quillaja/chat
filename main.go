package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	meProfile := flag.String("profile", "", "profile")
	contactsFile := flag.String("contacts", "", "contacts")
	privKeyFile := flag.String("key", "", "private key")
	flag.Parse()

	// log stuff
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

	app := NewReplApp(*meProfile, *contactsFile, *privKeyFile, Color(os.Stdout, Green))
	app.Run()

	// doing a "bot"
	// add "bot.Read()" to line processor
	// botR, botW := io.Pipe()
	// bot := NewConsole(botR)
	// go bot.Run()
	// go func() {
	// 	console.Prompt(false)
	// 	botW.Write([]byte("me\n"))
	// 	botW.Write([]byte("ip\n"))
	// 	console.Prompt(true)
	// }()
	// time.Sleep(100 * time.Millisecond) // give 'bot' chance to run

	time.Sleep(500 * time.Millisecond)
	log.Println("exiting program")
}

// Color constants.
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

// Color wraps a writer so subsequent writes are surrounded with
// ANSI escape codes to color console output.
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

// GetIP gets the client's external IP address using an external webservice.
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
