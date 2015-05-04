package main

import (
	"fmt"
	"github.com/nsf/termbox-go"
	"os"
	"regexp"
	"runtime"
	"time"
)

var (
	confPath    = os.Getenv("HOME") + "/.config/kome"
	accountPath = confPath + "/account.json"
	dbPath      = confPath + "/user.sqlite"
)

func stdErr(err error) {
	fmt.Fprintf(os.Stderr, "kome: %v\n", err)
}
func usage() {
	fmt.Fprintf(os.Stdout, "Usage: kome \x1b[4mURL or lv***\x1b[0m\n")
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	if len(os.Args) != 2 {
		usage()
		return
	}

	liveID := regexp.MustCompile(`lv\d+`).FindString(os.Args[1])
	if liveID == "" {
		usage()
		return
	}

	// load account
	account, err := LoadAccount(accountPath)
	if err != nil {
		stdErr(err)
		return
	}
	if err := account.HeartBeat(); err != nil {
		if err := account.Login(); err != nil {
			stdErr(err)
			return
		}
		if err := account.HeartBeat(); err != nil {
			stdErr(err)
			return
		}
		if err := account.SaveTo(accountPath); err != nil {
			stdErr(err)
			return
		}
	}

	// open and migrate user database
	// create user repo
	db, err := OpenWithMigrate(dbPath)
	if err != nil {
		stdErr(err)
	}
	repo := NewUserRepo(db)

	// load and connect live
	lv := NewLive(account, repo, liveID)
	if err := lv.LoadPlayerStatus(); err != nil {
		stdErr(err)
		return
	}
	if err := lv.Connect(time.Second * 5); err != nil {
		stdErr(err)
		return
	}
	defer lv.Close()

	// init termbox
	if err := termbox.Init(); err != nil {
		stdErr(err)
		return
	}
	defer termbox.Close()

	// create view and start kome!
	view := NewView(lv)
	view.Loop()
}
