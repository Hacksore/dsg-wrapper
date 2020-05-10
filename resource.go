package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
)

var (
	shuttingDown = false
	cleaningUp   = false
)

func cleanUp() {
	actualCleanup := func() {
		err := autogenResources.Delete()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error":            err,
				"autogenResources": autogenResources,
			}).Panic("wtf can't delete?? please delete the automatically allocated resources manually")
		}
	}

	if !cleaningUp {
		cleaningUp = true
		actualCleanup()
	}
}

func setupExitAndTerminationCleaner() {
	onExit := func() {
		logrus.Info("termination event received, cleaning up resources")
		cleanUp()
	}

	logrus.RegisterExitHandler(onExit)

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGKILL, syscall.SIGABRT, syscall.SIGTERM)
	go func() {
		<-c
		onExit()
		os.Exit(1)
	}()
}
