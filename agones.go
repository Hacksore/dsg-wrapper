package main

import (
	"strconv"
	"time"

	"agones.dev/agones/pkg/sdk"
	sdk2 "agones.dev/agones/sdks/go"
	"github.com/sirupsen/logrus"
)

func setupAgonesStateWatcher() {
	actualShutdown := func() {
		logrus.Info("Agones told me to go to sleep. Arrivederci😇")
		cleanUp()
	}

	err := GlobalSDKState.Instance.WatchGameServer(func(gs *sdk.GameServer) {
		if gs.GetStatus().State == "Shutdown" && !shuttingDown {
			shuttingDown = true
			actualShutdown()
		}
	})
	if err != nil {
		logrus.WithError(err).Fatal("unable to watch game server status")
	}
}

func connectToAgones() {
	// bail out here cause we don't need to connect to agones
	if GlobalArguments.SkipAgones {
		logrus.Warn("Agones connection is skipped ;(")
		return
	}

	tick := 1
	for !GlobalSDKState.ConnectionEstablished {
		logrus.Info("connecting to Agones SDK")
		instance, err := sdk2.NewSDK()

		if err != nil {
			logrus.Warn("can't connect to Agones SDK")
		} else {
			logrus.Info("successfully connected to the Agones SDK!")
			GlobalSDKState.Instance = instance
			GlobalSDKState.ConnectionEstablished = true
		}

		// exponential backoff
		tick *= 2
		<-time.Tick(time.Duration(tick) * time.Second)
	}

	redirectPorts := func() {
		conf, err := GlobalSDKState.Instance.GameServer()
		if err == nil {
			for _, port := range conf.Status.Ports {
				name, value := port.GetName(), int(port.GetPort())

				logrus.WithFields(logrus.Fields{
					"name":  name,
					"value": value,
				}).Debug("traversed port entry from Agones SDK")

				for _, port := range GlobalArguments.PortMaps {
					if port.Name == name {
						patchGlobalVariable(port.To, strconv.Itoa(value))
					}
				}
			}
		}
	}

	checkHealth := func() {
		logrus.Info("started health checking")
		tick = 2
		for {
			err := GlobalSDKState.Instance.Health()
			if err != nil {
				logrus.WithError(err).Fatal("could not send health ping")
			}
			<-time.Tick(time.Duration(tick) * time.Second)
		}
	}

	redirectPorts()
	go checkHealth()
}
