package main

import (
	"fmt"
	"github.com/mattn/go-runewidth"
	"github.com/nsf/termbox-go"
	"sort"
	"strconv"
	"strings"
	"time"
)

const chainThreshold = 500 * 1000 * 1000

type View struct {
	quit   bool
	width  int
	height int
	top    int
	ptr    int
	live   *Live
	komes  []Chat
	cmd    []rune
	prev   int64
	chain  []rune
}

func NewView(live *Live) *View {
	w, h := termbox.Size()
	return &View{
		width:  w,
		height: h,
		top:    0,
		ptr:    0,
		live:   live,
	}
}

func (v *View) Loop() {
	evCh := make(chan termbox.Event)
	go func() {
		for {
			evCh <- termbox.PollEvent()
		}
	}()

	// first draw
	v.updateView()

	tick := time.Tick(time.Second / 2)
	for {
		select {
		case <-tick:
		case ev := <-evCh:
			if ev.Type == termbox.EventKey && ev.Key == termbox.KeyCtrlC {
				return
			}
			v.updateEvent(ev)
		case kome := <-v.live.KomeCh:
			v.updateKome(kome)
		}

		if v.quit {
			break
		}
		v.updateView()
	}
}

func (v *View) updateEvent(ev termbox.Event) {
	switch ev.Type {
	case termbox.EventResize:
		v.width, v.height = ev.Width, ev.Height
		v.fixPtr()
	case termbox.EventKey:
		now := time.Now().UnixNano()
		switch {
		case ev.Ch == 0:
			v.chain = nil
			v.prev = 0
		case now-v.prev > chainThreshold:
			v.chain = []rune{ev.Ch}
			v.prev = now
		default:
			v.chain = append(v.chain, ev.Ch)
			v.prev = now
		}

		if len(v.cmd) != 0 {
			// cmd now
			switch ev.Key {
			case termbox.KeyEsc:
				v.cmd = nil
			case termbox.KeyEnter:
				v.execCommand()
			case termbox.KeyBackspace, termbox.KeyBackspace2:
				if len(v.cmd) > 1 || v.cmd[0] != 'i' {
					v.cmd = v.cmd[0 : len(v.cmd)-1]
				}
			case termbox.KeySpace:
				v.cmd = append(v.cmd, ' ')
			default:
				if ev.Ch != 0 {
					v.cmd = append(v.cmd, ev.Ch)
				}
			}
			return
		}

		switch ev.Ch {
		case 'i', ':':
			v.cmd = append(v.cmd, ev.Ch)
		case 'j':
			v.ptr++
			v.fixPtr()
		case 'k':
			v.ptr--
			v.fixPtr()
		case 'G':
			c := string(v.chain)
			if len(c) > 1 && c[len(c)-1] == 'G' {
				n, err := strconv.ParseInt(c[0:len(c)-1], 10, 32)
				if err == nil {
					v.jumpTo(int(n))
					break
				}
			}

			v.ptr = len(v.komes) - 1
			v.fixPtr()
		case 'g':
			if string(v.chain) == "gg" {
				v.ptr = 0
				v.fixPtr()
			}
		}
	}
}

func (v *View) calcEnd() int {
	h := v.height - 2
	if h < 1 {
		h = 1
	}

	end := v.top + h
	if end > len(v.komes) {
		end = len(v.komes)
	}
	return end
}

func (v *View) fixPtr() {
	if len(v.komes) == 0 {
		v.top = 0
		v.ptr = 0
		return
	}

	if v.ptr < 0 {
		v.ptr = 0
	}
	if v.ptr >= len(v.komes) {
		v.ptr = len(v.komes) - 1
	}

	if v.ptr < v.top {
		v.top = v.ptr
		return
	}

	end := v.calcEnd()
	if v.ptr >= end {
		v.top += v.ptr - end + 1
		return
	}
}

func (v *View) jumpTo(n int) {
	i := sort.Search(len(v.komes), func(i int) bool { return v.komes[i].No >= n })
	if i < len(v.komes) && v.komes[i].No == n {
		v.ptr = i
		v.fixPtr()
	}
}

func (v *View) execCommand() {
	defer func() {
		v.cmd = nil
	}()

	cmd := string(v.cmd)

	// quit
	if cmd == ":q" {
		v.quit = true
		return
	}

	// send 184 kome
	if strings.HasPrefix(cmd, ":184 ") {
		comment := cmd[5:]
		v.live.SendKome(comment, true)
		return
	}

	// send raw kome
	if strings.HasPrefix(cmd, "i") {
		comment := cmd[1:]
		v.live.SendKome(comment, false)
		return
	}

	// :23 -> jump to 23kome
	n, err := strconv.ParseInt(cmd[1:], 10, 32)
	if err == nil {
		v.jumpTo(int(n))
	}
}

func (v *View) updateKome(kome Chat) {
	if len(v.komes) == 0 {
		v.top = 0
		v.ptr = 0
		v.komes = append(v.komes, kome)
		return
	}

	end := v.calcEnd()
	if end == len(v.komes) {
		h := v.height - 2
		if h < 1 {
			h = 1
		}
		if end-v.top+1 > h {
			v.top++
			if v.ptr < v.top {
				v.ptr = v.top
			}
		}
	}

	v.komes = append(v.komes, kome)
}

func (v *View) updateView() {
	termbox.HideCursor()
	nowCmd := len(v.cmd) != 0

	// line view
	if len(v.komes) > 0 && v.height > 2 {
		end := v.calcEnd()

		noPadFormat := func() string {
			last := v.komes[end-1]
			noStr := fmt.Sprintf("%d", last.No)
			return fmt.Sprintf("%%0%dd", len(noStr))
		}()
		maxUserNameLen := func() int {
			maxLen := 0
			for _, kome := range v.komes[v.top:end] {
				l := stringWidth(kome.User.Name)
				if !kome.IsRawUser {
					l = 3
				}
				if l > maxLen {
					maxLen = l
				}
			}
			return maxLen
		}()

		y := 0
		for i := v.top; i < end; i++ {
			bg := termbox.ColorDefault
			if i == v.ptr {
				bg = termbox.ColorGreen
			}

			x := 0
			{
				// no
				fg := termbox.ColorBlue
				if i == v.ptr {
					fg = termbox.ColorDefault
				}

				no := fmt.Sprintf(noPadFormat, v.komes[i].No)
				for _, c := range no {
					termbox.SetCell(x, y, c, fg, bg)
					x++
				}
			}

			termbox.SetCell(x, y, ' ', termbox.ColorDefault, bg)
			x++

			{
				//time
				fg := termbox.ColorYellow
				if i == v.ptr {
					fg = termbox.ColorDefault
				}

				st := time.Unix(v.live.Status.Stream.StartTime, 0)
				tm := time.Unix(v.komes[i].Date, 0)
				dif := tm.Sub(st)
				line := fmt.Sprintf("%02d:%02d", int(dif.Minutes()), int(dif.Seconds())%60)
				for _, c := range line {
					termbox.SetCell(x, y, c, fg, bg)
					x++
				}
			}

			termbox.SetCell(x, y, ' ', termbox.ColorDefault, bg)
			x++

			{
				// userName
				fg := termbox.ColorGreen
				userName := v.komes[i].User.Name

				if !v.komes[i].IsRawUser {
					fg = termbox.ColorYellow
					userName = "184"
				}
				if i == v.ptr {
					fg = termbox.ColorDefault
				}

				l := 0
				for _, c := range userName {
					termbox.SetCell(x, y, c, fg, bg)
					w := runewidth.RuneWidth(c)
					x += w
					l += w
				}
				for ; l < maxUserNameLen; l++ {
					termbox.SetCell(x, y, ' ', fg, bg)
					x++
				}
			}

			termbox.SetCell(x, y, ' ', termbox.ColorDefault, bg)
			x++

			for _, c := range v.komes[i].Comment {
				termbox.SetCell(x, y, c, termbox.ColorDefault, bg)
				x += width(c)
			}
			for ; x < v.width; x++ {
				termbox.SetCell(x, y, ' ', termbox.ColorDefault, bg)
			}

			if i == v.ptr && !nowCmd {
				termbox.SetCursor(v.width-1, y)
			}
			y++
		}
		for ; y < v.height-2; y++ {
			for x := 0; x < v.width; x++ {
				termbox.SetCell(x, y, ' ', termbox.ColorDefault, termbox.ColorDefault)
			}
		}
	}

	// info view
	if v.height > 1 {
		left := fmt.Sprintf("[%s] %s", v.live.LiveID, v.live.Status.Stream.Title)

		par := 0
		if len(v.komes) > 0 {
			par = v.calcEnd() * 100 / len(v.komes)
		}

		end := time.Unix(v.live.Status.Stream.EndTime, 0)
		dif := end.Sub(time.Now())

		right := fmt.Sprintf("%02d:%02d | %d%%", int(dif.Minutes()), int(dif.Seconds())%60, par)

		y := v.height - 2
		x := 0
		for _, c := range left {
			termbox.SetCell(x, y, c, termbox.ColorDefault, termbox.ColorBlue)
			x += width(c)
		}

		mid := v.width - x - len(right)
		if mid > 0 {
			for i := 0; i < mid; i++ {
				termbox.SetCell(x, y, ' ', termbox.ColorDefault, termbox.ColorBlue)
				x++
			}
			for _, c := range right {
				termbox.SetCell(x, y, c, termbox.ColorDefault, termbox.ColorBlue)
				x++
			}
		}

		for ; x < v.width; x++ {
			termbox.SetCell(x, y, ' ', termbox.ColorDefault, termbox.ColorBlue)
		}
	}

	// cmd view
	if v.height > 0 {
		y := v.height - 1
		x := 0

		cmd := string(v.cmd)
		if len(cmd) > 0 && cmd[0] == 'i' {
			cmd = "send: " + cmd[1:]
		}
		for _, c := range cmd {
			termbox.SetCell(x, y, c, termbox.ColorGreen, termbox.ColorDefault)
			x += width(c)
		}
		if nowCmd {
			termbox.SetCursor(x, y)
		}
		for ; x < v.width; x++ {
			termbox.SetCell(x, y, ' ', termbox.ColorDefault, termbox.ColorDefault)
		}
	}

	termbox.Flush()
}

func stringWidth(s string) int {
	w := 0
	for _, c := range s {
		w += width(c)
	}
	return w
}

func width(c rune) int {
	w := runewidth.RuneWidth(c)
	if w == 0 || w == 2 && runewidth.IsAmbiguousWidth(c) {
		w = 1
	}
	return w
}
