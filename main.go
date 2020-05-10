package main

import (
	"os"

	sdk "agones.dev/agones/sdks/go"
	log "github.com/sirupsen/logrus"
)

type (
	PortMap struct {
		Name string `toml:"name"`
		Type string `toml:"type"`
		To   string `toml:"to"`
	}

	GlobalArgumentsBody struct {
		AppID          *int      `arg:"-a,--app-id" help:"Sets the App ID of the dedicated server of the game" toml:"appID"`
		BinPath        string    `arg:"-i,--bin-path,required" help:"Path to server binary/script"`
		SearchString   string    `arg:"-s,--search-string,required" help:"String to search for ready state"`
		Debug          bool      `arg:"--debug,env:DEBUG" default:"false" help:"Enables debug mode" toml:"debug"`
		SkipAgones     bool      `arg:"--skip-agones,env:SKIP_AGONES" default:"false" help:"Skip agones connection" toml:"skipAgones"`
		ConfigFile     string    `arg:"-c,--config-file,env:CONFIG" default:"config.toml" help:"Location of the TOML config file. It will merge with the command arguments"`
		SteamWebApiKey *string   `arg:"--steam-web-api-key,env:STEAM_WEB_API_KEY" help:"Set the Steam Web API key" toml:"steamWebApiKey"`
		AutogenGSLT    bool      `arg:"--auto-gen-gslt,env:AUTOGEN_GSLT" default:"false" help:"Automatically generates GSLT token? (Steam Web API key required)" toml:"autogenGSLT"`
		PortMaps       []PortMap `arg:"--port-maps" help:"Add an entry of ports to map (entries in TOML object format)" toml:"portMaps"`
		Quiet          bool      `arg:"-q,--quiet,env:QUIET" default:"false" help:"If enabled, disables standard output redirection of child process" toml:"quiet"`
	}
)

var (
	GlobalSDKState struct {
		Instance              *sdk.SDK
		ServerReadySent       bool
		ConnectionEstablished bool
	}

	GlobalArguments GlobalArgumentsBody
)

func (GlobalArgumentsBody) Description() string {
	return "Connect to Agones (optionally) and spawn a child process, usually a dedicated server\nExample: dsg-wrapper -i /home/steam/start.sh -s \"VAC mode\""
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
	setupExitAndTerminationCleaner()

	parseConfig()

	if !GlobalArguments.SkipAgones {
		connectToAgones()

		if GlobalSDKState.ConnectionEstablished {
			setupAgonesStateWatcher()
		}
	}

	spawnProcess()
}
