package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml"

	log "github.com/sirupsen/logrus"
)

func patchGlobalVariable(name string, value string) {
	log.WithFields(log.Fields{
		"name":  name,
		"value": value,
	}).Info("patching global variable")
	if err := os.Setenv(name, value); err != nil {
		log.WithError(err).Error("cannot set global variable")
	}
}

func (n *PortMap) UnmarshalText(b []byte) error {
	s := string(b)
	leftBrace := strings.Index(s, "{")
	if leftBrace == -1 {
		return fmt.Errorf("missing { in %s", s)
	}
	rightBrace := strings.Index(s, "}")
	if rightBrace == -1 {
		return fmt.Errorf("missing } in %s", s)
	}
	var tmp struct {
		Tmp PortMap
	}
	str := fmt.Sprintf("tmp=%s", s[leftBrace:rightBrace+1])
	if err := toml.Unmarshal([]byte(str), &tmp); err != nil {
		return err
	}

	*n = tmp.Tmp
	return nil
}
