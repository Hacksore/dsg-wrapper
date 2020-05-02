package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	sdk "agones.dev/agones/sdks/go"
)

type interceptor struct {
	forward   io.Writer
	intercept func(p []byte)
}

// Write will intercept the incoming stream, and forward
// the contents to its `forward` Writer.
func (i *interceptor) Write(p []byte) (n int, err error) {
	if i.intercept != nil {
		i.intercept(p)
	}

	return i.forward.Write(p)
}

// main intercepts the stdout of the gameserver and uses it
// to determine if the game server is ready or not.

// We can run game like this:
// dsg-wrapper -i <path to game> -s "<stdout text to mark dsg ready>"
// exmple dsg-wrapper -i /home/steam/start.sh -s "VAC mode"
func main() {

	binPath := flag.String("i", "", "path to server_linux.sh")
	searchString := flag.String("s", "", "String to search for ready state")
	flag.Parse()


	fmt.Println(">>> Connecting to Agones with the SDK")
	s, err := sdk.NewSDK()

	// TODO: need to add retrying
	if err != nil {
		log.Fatalf(">>> Could not connect to sdk, need to try again: %v", err)
	}

	fmt.Println(">>> Starting health checking")
	go doHealth(s)

	fmt.Println(">>> Starting wrapper!")
	fmt.Printf(">>> Path to server binary/script: %s \n", *binPath)

	cmd := exec.Command(*binPath) // #nosec
	cmd.Stderr = &interceptor{forward: os.Stderr}
	cmd.Stdout = &interceptor{
		forward: os.Stdout,
		intercept: func(p []byte) {

			str := strings.TrimSpace(string(p))

			if strings.Contains(str, *searchString) {
				fmt.Printf(">>> Moving to READY: %s \n", str)
				err = s.Ready()
				if err != nil {
					log.Fatalf("Could not send ready message")
				}
			}
		}}

	err = cmd.Start()
	if err != nil {
		log.Fatalf(">>> Error starting %v", err)
	}
	err = cmd.Wait()
	log.Fatal(">>> Game server shutdown unexpectantly", err)
}

// doHealth sends the regular Health Pings
func doHealth(sdk *sdk.SDK) {
	tick := time.Tick(2 * time.Second)
	for {
		err := sdk.Health()
		if err != nil {
			log.Fatalf("[wrapper] Could not send health ping, %v", err)
		}
		<-tick
	}
}