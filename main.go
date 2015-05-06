package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"runtime"
	"time"

	"github.com/nsf/termbox-go"
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

var App *Application

type Application struct {
	live *Live
	view *View

	logger *log.Logger
}

func NewLogger(logFile string) *log.Logger {
	var f io.Writer
	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		f = ioutil.Discard
	}

	return log.New(f, "*** ", log.LstdFlags|log.Llongfile)
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	logFile := flag.String("log", "", "log file path")
	flag.Parse()

	// app instance
	App = &Application{}
	App.logger = NewLogger(*logFile)

	if len(flag.Args()) != 1 {
		usage()
		return
	}

	liveID := regexp.MustCompile(`lv\d+`).FindString(flag.Arg(0))
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
	App.live = lv

	// init termbox
	if err := termbox.Init(); err != nil {
		stdErr(err)
		return
	}
	defer termbox.Close()

	// create view and start kome!
	view := NewView(lv)
	view.Loop()

	App.live.Close()
}
