package main

import (
	"io/ioutil"
	"os"

	"github.com/alexflint/go-arg"
	"github.com/imdario/mergo"
	"github.com/pelletier/go-toml"
	"github.com/sirupsen/logrus"
	"github.com/stevefan1999-personal/steam-gameserver-token-api/steam"
)

func parseConfig() {
	arg.MustParse(&GlobalArguments)

	parseConfigAndMergeIfPossible()
	disableOptionsIfPrerequisiteNotMet()

	if GlobalArguments.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	logrus.WithField("config", GlobalArguments).Debug("config")
}

func parseConfigAndMergeIfPossible() {
	fileSrc, err := ioutil.ReadFile(GlobalArguments.ConfigFile)
	if err != nil {
		logrus.WithError(err).Error("cannot read the config file")
		return
	}

	var config GlobalArgumentsBody
	if err := toml.Unmarshal(fileSrc, &config); err != nil {
		logrus.WithError(err).Error("cannot parse the config file")
	} else {
		if err := mergo.Merge(&GlobalArguments, config, mergo.WithOverride); err != nil {
			logrus.WithError(err).Fatal("cannot merge the config file")
		}
	}
}

func disableOptionsIfPrerequisiteNotMet() {
	GlobalArguments.AutogenGSLT = func() bool {
		if GlobalArguments.AutogenGSLT {
			if GlobalArguments.SteamWebApiKey == nil {
				logrus.Warn("Steam Web API key not set. Disabling automatic GSLT generation")
				return false
			}
			if GlobalArguments.AppID == nil {
				logrus.Warn("Steam App ID not set. Disabling automatic GSLT generation")
				return false
			}
			if value := os.Getenv("STEAM_ACCOUNT"); value != "" {
				logrus.Warn("GSLT token already set. Disabling automatic GSLT generation no matter what")
				return false
			}
		}
		return GlobalArguments.AutogenGSLT
	}()

	if GlobalArguments.AutogenGSLT {
		steam.SetSteamAPIKey(*GlobalArguments.SteamWebApiKey)
	}

}
