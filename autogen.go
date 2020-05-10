package main

import (
	"errors"
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

func (this *GSLTState) GenerateToken(appId int, memo string, forceAllocate bool) error {
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

func (this *GSLTState) DeleteToken() error {
	if this.Token == "" {
		return errors.New("not allocated")
	}
	return steam.DeleteAccount(this.SteamId)
}

func (this *AutogenResources) DeleteResources() error {
	err := this.GSLT.DeleteToken()
	if err != nil {
		return err
	}
	return err
}
