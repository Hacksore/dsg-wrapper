package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/creack/pty"
	"github.com/inetaf/tcpproxy"
	"github.com/sirupsen/logrus"
)

func spawnProcess() {
	generateResources()

	logrus.Info("starting wrapper target")
	logrus.WithField("binPath", GlobalArguments.BinPath).Info("path to server binary/script")

	cmd := exec.Command(GlobalArguments.BinPath)
	cmd.Env = os.Environ()

	tty, err := pty.Start(cmd)
	if err != nil {
		logrus.WithError(err).Fatal("unable to spawn the child process")
	}

	defer tty.Close()

	startScanning := func() {
		scanner := bufio.NewScanner(tty)
		for scanner.Scan() {
			p := scanner.Text()
			if !GlobalArguments.Quiet {
				logrus.WithField("value", p).Info("child printed")
			}

			if GlobalSDKState.Instance == nil || GlobalArguments.SkipAgones || GlobalSDKState.ServerReadySent {
				continue
			}

			str := strings.TrimSpace(p)
			foundString := strings.Contains(str, GlobalArguments.SearchString)
			if !foundString {
				continue
			}

			logrus.Info("moving to READY state")

			if err := GlobalSDKState.Instance.Ready(); err != nil {
				logrus.Fatal("could not send ready message")
			} else {
				GlobalSDKState.ServerReadySent = true
			}
		}
	}
	startCopyStdin := func() {
		_, err := io.Copy(tty, os.Stdin)
		if err != nil {
			logrus.WithError(err).Error("cannot copy standard input to the child process")
		}
	}
	startProxy := func() {
		var p tcpproxy.Proxy
		p.AddRoute(fmt.Sprintf(":%s", os.Getenv("RCON_PORT")), tcpproxy.To(fmt.Sprintf("127.0.0.1:%s", os.Getenv("PORT"))))
		logrus.Fatal(p.Run())
	}
	waitUntilServerCrash := func() {
		err = cmd.Wait()

		logrus.WithError(err).Error("game server shutdown unexpectedly")
		if GlobalSDKState.ConnectionEstablished {
			logrus.Info("telling Agones to take me down ;(")
			err := GlobalSDKState.Instance.Shutdown()
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"error":            err,
					"autogenResources": autogenResources,
				}).Panic("wtf can't shutdown?? please delete the automatically allocated resources manually")
				cleanUp()
			}
		} else {
			logrus.WithError(err).Fatal("goodbye")
		}
	}

	go startScanning()
	go startCopyStdin()
	go startProxy()
	waitUntilServerCrash()
}
