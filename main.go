package main

import (
	"fmt"
	"os/user"
	"os"
	"regexp"
	"time"
	"github.com/nsf/termbox-go"
	"runtime"
)

const usage = "Usage: kome \x1b[4mURL or lv***\x1b[0m\n"

func main(){
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
	ctxPath := u.HomeDir + "/.config/kome/context.json"

	ctx, err := LoadContext(ctxPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kome: %v\n", err)
		return
	}

	if !ctx.HeartBeat() {
		if !ctx.Login() || !ctx.HeartBeat() {
			fmt.Fprintf(os.Stderr, "kome: failed to login\n")
			return
		}
		if err := ctx.SaveTo(ctxPath); err != nil {
			fmt.Fprintf(os.Stderr, "kome: %v\n", err)
			return
		}
	}

	lv := NewNicoLive(ctx.NewClient(), liveID)
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
	go func(){
		for {
			evCh <- termbox.PollEvent()
		}
	}()

	view := NewView(lv)

	loop:
	for {
		select {
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