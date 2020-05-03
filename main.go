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
	"github.com/creack/pty"
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

var sdkInstance *sdk.SDK
var skipAgonesConnection bool
var sdkConnectionEstablished bool

// We can run game like this:
// dsg-wrapper -i <path to game> -s "<text found in stdout to mark dsg ready>"
// exmple dsg-wrapper -i /home/steam/start.sh -s "VAC mode"
func main() {
	// try connecting to agones if needed
	connectToAgones()

	// spawn process so we can introspect stdout and pass stdin to the downstream proc
	spawnProcess()
}

func spawnProcess() {

	binPath := flag.String("i", "", "path to server binary/script")
	searchString := flag.String("s", "", "String to search for ready state")

	flag.Parse()

	fmt.Println(">>> Starting wrapper!")
	fmt.Printf(">>> Path to server binary/script: %s \n", *binPath)

	cmd := exec.Command(*binPath)
	cmd.Stderr = &interceptor{forward: os.Stderr}
	cmd.Stdout = &interceptor{
		forward: os.Stdout,
		intercept: func(p []byte) {

			str := strings.TrimSpace(string(p))

			foundString := strings.Contains(str, *searchString)
			fmt.Printf("Found string: %v\n", foundString)

			// if we skip connection to agones bail out
			if skipAgonesConnection {
				return
			}

			if foundString {
				fmt.Printf(">>> Moving to READY as we found '%v'\n", *searchString)
				err := sdkInstance.Ready()

				if err != nil {
					log.Fatalf("Could not send ready message")
				}
			}
		}}

	tty, err := pty.Start(cmd)
	if err != nil {
		log.Fatalf(">>> Error starting %v", err)
	}

	defer tty.Close()

	go func() {
		io.Copy(tty, os.Stdin)
	}()

	err = cmd.Wait()
	log.Fatal(">>> Game server shutdown unexpectantly", err)
}

func connectToAgones() {
	_, envFlag := os.LookupEnv("SKIP_AGONES")

	fmt.Printf(">>> Skip Agones: %v \n", envFlag)

	// bail out here cause we don't need to connect to agones
	if envFlag {
		skipAgonesConnection = true
		return
	}

	// try reconnect in case something goes wrong
	tick := time.Tick(2 * time.Second)
	for !sdkConnectionEstablished {

		fmt.Println(">>> Connecting to Agones with the SDK")
		ref, err := sdk.NewSDK()
		sdkInstance = ref

		sdkConnectionEstablished = true

		if err != nil {
			fmt.Println(">>> Can't connect to Agones with the SDK")
		}

		<-tick
	}

	// spwan health checking go routine
	fmt.Println(">>> Starting health checking")
	go doHealth()

}

// doHealth sends the regular Health Pings
func doHealth() {
	tick := time.Tick(2 * time.Second)
	for {
		err := sdkInstance.Health()
		if err != nil {
			log.Fatalf("[wrapper] Could not send health ping, %v", err)
		}
		<-tick
	}
}
