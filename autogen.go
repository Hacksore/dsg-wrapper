package main

import (
	"errors"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/stevefan1999-personal/steam-gameserver-token-api/steam"
)

type (
	GSLTState struct {
		Token   string
		SteamId string
	}

	AutogenResources struct {
		GSLT *GSLTState
	}
)

func (this *GSLTState) New(appId int, memo string, forceAllocate bool) error {
	if this.Token != "" && !forceAllocate {
		return errors.New("already allocated")
	}
	acc, err := steam.CreateAccount(appId, memo)
	if err == nil && acc.LoginToken != "" {
		this.Token = acc.LoginToken
		this.SteamId = acc.SteamID
	}
	return err
}

func (this *GSLTState) Delete() error {
	if this.Token == "" {
		return errors.New("not allocated")
	}
	return steam.DeleteAccount(this.SteamId)
}

func (this *AutogenResources) Delete() error {
	err := this.GSLT.Delete()
	if err != nil {
		return err
	}
	return err
}

var autogenResources AutogenResources

func generateResources() {
	if GlobalArguments.AutogenGSLT {
		logrus.Info("generating GSLT token and exposing it as environmental variables")
		memo := func() *string {
			if !GlobalSDKState.ConnectionEstablished {
				return nil
			}

			if hostname, err := os.Hostname(); err == nil {
				return &hostname
			} else {
				logrus.Info("cannot determine current host name, leaving the generated key memo to none")
			}
			return nil
		}()

		gsltState := new(GSLTState)
		err := gsltState.New(*GlobalArguments.AppID, func() string {
			if memo != nil {
				return *memo
			}
			return ""
		}(), false)
		if err != nil {
			logrus.WithError(err).Warn("cannot generate GSLT token")
		} else {
			if err := os.Setenv("STEAM_ACCOUNT", gsltState.Token); err != nil {
				logrus.WithError(err).Panic("cannot set the GSLT token")
			}
			autogenResources.GSLT = gsltState
		}

		if autogenResources.GSLT != nil {
			logrus.WithField("token", autogenResources.GSLT.Token).
				WithField("steamID", autogenResources.GSLT.SteamId).
				Info("generated ephemeral GSLT token")

		}
	}
}
