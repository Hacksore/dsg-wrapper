package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/creack/pty"
	"github.com/imdario/mergo"
	"github.com/inetaf/tcpproxy"
	"github.com/pelletier/go-toml"
	"github.com/stevefan1999-personal/steam-gameserver-token-api/steam"

	sdk2 "agones.dev/agones/pkg/sdk"
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

	autogenResources AutogenResources

	GlobalArguments GlobalArgumentsBody

	shuttingDown bool = false
	cleaningUp   bool = false
)

func (GlobalArgumentsBody) Description() string {
	return "Connect to Agones (optionally) and spawn a child process, usually a dedicated server\nExample: dsg-wrapper -i /home/steam/start.sh -s \"VAC mode\""
}

func cleanUp() {
	if !cleaningUp {
		cleaningUp = true

		err := autogenResources.DeleteResources()
		if err != nil {
			log.WithFields(log.Fields{
				"error":            err,
				"autogenResources": autogenResources,
			}).Panic("wtf can't delete?? please delete the automatically allocated resources manually")
		}
	}
}

func setupExitAndTerminationCleaner() {
	onExit := func() {
		log.Info("termination event received, cleaning up resources")
		cleanUp()
	}

	log.RegisterExitHandler(onExit)

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGKILL, syscall.SIGABRT, syscall.SIGTERM)
	go func() {
		<-c
		onExit()
		os.Exit(1)
	}()
}

func setupAgonesStateWatcher() {
	err := GlobalSDKState.Instance.WatchGameServer(func(gs *sdk2.GameServer) {
		if gs.GetStatus().State == "Shutdown" && !shuttingDown {
			shuttingDown = true
			log.Info("Agones told me to go to sleep. Arrivederci😇")
			cleanUp()
		}
	})
	if err != nil {
		log.WithError(err).Fatal("unable to watch game server status")
	}
}

func parseConfig() {
	arg.MustParse(&GlobalArguments)

	parseConfigAndMergeIfPossible()
	disableOptionsIfPrerequisiteNotMet()

	if GlobalArguments.Debug {
		log.SetLevel(log.DebugLevel)
	}
	log.WithField("config", GlobalArguments).Debug("config")
}

func parseConfigAndMergeIfPossible() {
	fileSrc, err := ioutil.ReadFile(GlobalArguments.ConfigFile)
	if err != nil {
		log.WithError(err).Error("cannot read the config file")
		return
	}

	var config GlobalArgumentsBody
	if err := toml.Unmarshal(fileSrc, &config); err != nil {
		log.WithError(err).Error("cannot parse the config file")
	} else {
		if err := mergo.Merge(&GlobalArguments, config, mergo.WithOverride); err != nil {
			log.WithError(err).Fatal("cannot merge the config file")
		}
	}
}

func disableOptionsIfPrerequisiteNotMet() {
	GlobalArguments.AutogenGSLT = func() bool {
		if GlobalArguments.AutogenGSLT {
			if GlobalArguments.SteamWebApiKey == nil {
				log.Warn("Steam Web API key not set. Disabling automatic GSLT generation")
				return false
			}
			if GlobalArguments.AppID == nil {
				log.Warn("Steam App ID not set. Disabling automatic GSLT generation")
				return false
			}
			if value := os.Getenv("STEAM_ACCOUNT"); value != "" {
				log.Warn("GSLT token already set. Disabling automatic GSLT generation no matter what")
				return false
			}
		}
		return GlobalArguments.AutogenGSLT
	}()

	if GlobalArguments.AutogenGSLT {
		steam.SetSteamAPIKey(*GlobalArguments.SteamWebApiKey)
	}

}

func generateResources() {
	if GlobalArguments.AutogenGSLT {
		log.Info("generating GSLT token and exposing it as environmental variables")
		memo := func() string {
			if !GlobalSDKState.ConnectionEstablished {
				return ""
			}

			if hostname, err := os.Hostname(); err == nil {
				return hostname
			} else {
				log.Info("cannot determine current host name, leaving the generated key memo to none")
			}
			return ""
		}()

		gslt := new(GSLTState)
		err := gslt.GenerateToken(*GlobalArguments.AppID, memo, false)
		if err != nil {
			log.WithError(err).Warn("cannot generate GSLT token")
		} else {
			if err := os.Setenv("STEAM_ACCOUNT", gslt.Token); err != nil {
				log.WithError(err).Panic("cannot set the GSLT token")
			}
			autogenResources.GSLT = gslt
		}

		if autogenResources.GSLT != nil {
			log.
				WithField("token", autogenResources.GSLT.Token).
				WithField("steamID", autogenResources.GSLT.SteamId).
				Info("generated ephemeral GSLT token")

		}
	}
}

func spawnProcess() {
	generateResources()

	log.Info("starting wrapper target")
	log.WithField("binPath", GlobalArguments.BinPath).Info("path to server binary/script")

	cmd := exec.Command(GlobalArguments.BinPath)
	cmd.Env = os.Environ()

	tty, err := pty.Start(cmd)
	if err != nil {
		log.WithError(err).Fatal("unable to spawn the child process")
	}

	defer tty.Close()

	startScanning := func() {
		scanner := bufio.NewScanner(tty)
		for scanner.Scan() {
			p := scanner.Text()
			if !GlobalArguments.Quiet {
				log.WithField("value", p).Info("child printed")
			}

			if GlobalSDKState.Instance == nil || GlobalArguments.SkipAgones || GlobalSDKState.ServerReadySent {
				continue
			}

			str := strings.TrimSpace(p)
			foundString := strings.Contains(str, GlobalArguments.SearchString)
			if !foundString {
				continue
			}

			log.Info("moving to READY state")

			if err := GlobalSDKState.Instance.Ready(); err != nil {
				log.Fatal("could not send ready message")
			} else {
				GlobalSDKState.ServerReadySent = true
			}
		}
	}
	startCopyStdin := func() {
		_, err := io.Copy(tty, os.Stdin)
		if err != nil {
			log.WithError(err).Error("cannot copy standard input to the child process")
		}
	}
	startProxy := func() {
		var p tcpproxy.Proxy
		p.AddRoute(fmt.Sprintf(":%s", os.Getenv("RCON_PORT")), tcpproxy.To(fmt.Sprintf("127.0.0.1:%s", os.Getenv("PORT"))))
		log.Fatal(p.Run())
	}
	waitUntilServerCrash := func() {
		err = cmd.Wait()

		log.WithError(err).Error("game server shutdown unexpectedly")
		if GlobalSDKState.ConnectionEstablished {
			log.Info("telling Agones to take me down ;(")
			err := GlobalSDKState.Instance.Shutdown()
			if err != nil {
				log.WithFields(log.Fields{
					"error":            err,
					"autogenResources": autogenResources,
				}).Panic("wtf can't shutdown?? please delete the automatically allocated resources manually")
				cleanUp()
			}
		} else {
			log.WithError(err).Fatal("goodbye")
		}
	}

	go startScanning()
	go startCopyStdin()
	go startProxy()
	waitUntilServerCrash()
}

func connectToAgones() {
	// bail out here cause we don't need to connect to agones
	if GlobalArguments.SkipAgones {
		log.Warn("Agones connection is skipped ;(")
		return
	}

	tick := 1
	for !GlobalSDKState.ConnectionEstablished {
		log.Info("connecting to Agones SDK")
		instance, err := sdk.NewSDK()

		if err != nil {
			log.Warn("can't connect to Agones SDK")
		} else {
			log.Info("successfully connected to the Agones SDK!")
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

				log.WithFields(log.Fields{
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
		log.Info("started health checking")
		tick = 2
		for {
			err := GlobalSDKState.Instance.Health()
			if err != nil {
				log.WithError(err).Fatal("could not send health ping")
			}
			<-time.Tick(time.Duration(tick) * time.Second)
		}
	}

	redirectPorts()
	go checkHealth()
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
