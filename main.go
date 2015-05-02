package main

import (
	"fmt"
	"github.com/nsf/termbox-go"
	"os"
	"os/user"
	"regexp"
	"runtime"
	"time"
)

const usage = "Usage: kome \x1b[4mURL or lv***\x1b[0m\n"

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stdout, usage)
		return
	}

	liveID := regexp.MustCompile(`lv\d+`).FindString(os.Args[1])
	if liveID == "" {
		fmt.Fprintf(os.Stdout, usage)
		return
	}

	u, _ := user.Current()
	ctx, err := LoadContext(u.HomeDir + "/.config/kome")
	if err != nil {
		fmt.Fprintf(os.Stderr, "kome: %v\n", err)
		return
	}
	if !ctx.HeartBeat() {
		if !ctx.Login() || !ctx.HeartBeat() {
			fmt.Fprintf(os.Stderr, "kome: failed to login\n")
			return
		}
		if err := ctx.SaveConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "kome: %v\n", err)
			return
		}
	}

	lv := NewLive(ctx, liveID)
	if err := lv.GetPlayerStatus(); err != nil {
		fmt.Fprintf(os.Stderr, "kome: %v\n", err)
		return
	}
	if err := lv.Connect(time.Second * 5); err != nil {
		fmt.Fprintf(os.Stderr, "kome: %v\n", err)
		return
	}
	defer lv.Close()

	if err := termbox.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "kome: %v\n", err)
		return
	}
	defer termbox.Close()

	evCh := make(chan termbox.Event)
	go func() {
		for {
			evCh <- termbox.PollEvent()
		}
	}()

	view := NewView(lv)
	tick := time.Tick(time.Second / 2)

loop:
	for {
		select {
		case <-tick:
		case ev := <-evCh:
			if ev.Type == termbox.EventKey && ev.Key == termbox.KeyCtrlC {
				break loop
			}
			view.UpdateEvent(ev)
		case kome := <-lv.KomeCh:
			view.UpdateKome(kome)
		}

		if view.Quit {
			break
		}

		view.UpdateView()
	}
}
