package main
import (
	"github.com/nsf/termbox-go"
	"github.com/mattn/go-runewidth"
	"fmt"
	"strings"
)

type View struct {
	Quit   bool
	width  int
	height int
	top    int
	ptr    int
	live   *NicoLive
	komes  []Kome
	cmd    []rune
}

func NewView(live *NicoLive) *View {
	w, h := termbox.Size()
	return &View {
		width:  w,
		height: h,
		top:    0,
		ptr:    0,
		live:   live,
	}
}

func (v *View) UpdateEvent(ev termbox.Event) {
	switch ev.Type {
	case termbox.EventResize:
		v.width, v.height = ev.Width, ev.Height
		v.fixPtr()
	case termbox.EventKey:
		if len(v.cmd) != 0 {
			// cmd now
			switch ev.Key {
			case termbox.KeyEnter:
				v.execCommand()
			case termbox.KeyBackspace, termbox.KeyBackspace2:
				v.cmd = v.cmd[0:len(v.cmd)-1]
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
		case 'i', ':', '/':
			v.cmd = append(v.cmd, ev.Ch)
		case 'j':
			v.ptr++
			v.fixPtr()
		case 'k':
			v.ptr--
			v.fixPtr()
		case 'G':
			v.ptr = len(v.komes) - 1
			v.fixPtr()
		}
	}
}

func (v *View) calcEnd() int {
	end := v.top + (v.height - 2)
	if end > len(v.komes) {
		end = len(v.komes)
	}
	return end
}

func (v *View) fixPtr() {
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

func (v *View) execCommand() {
	defer func(){
		v.cmd = nil
	}()

	cmd := string(v.cmd)
	if cmd == ":q" {
		v.Quit = true
		return
	}

	if strings.HasPrefix(cmd, "i184 ") {
		comment := cmd[5:]
		v.live.SendKome(comment, true)
		return
	}
	if strings.HasPrefix(cmd, "i ") {
		comment := cmd[2:]
		v.live.SendKome(comment, false)
		return
	}
}

func (v *View) UpdateKome(kome Kome) {
	end := v.calcEnd()
	if end == len(v.komes) {
		if end - v.top + 1 > v.height - 2 {
			v.top++
			if v.ptr < v.top {
				v.ptr = v.top
			}
		}
	}

	v.komes = append(v.komes, kome)
}

func (v *View) UpdateView() {
	termbox.HideCursor()
	nowCmd := len(v.cmd) != 0

	// line view
	if len(v.komes) > 0 && v.height > 2 {
		end := v.calcEnd()

		noPadFormat := func() string {
			last := v.komes[end - 1]
			noStr := fmt.Sprintf("%d", last.No)
			return fmt.Sprintf("%%0%dd", len(noStr))
		}()
		maxUserIDLen := func() int {
			maxLen := 0
			for _, kome := range v.komes[v.top:end] {
				l := len(kome.UserID)
				if kome.Is184Comment() {
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
				// userID
				fg := termbox.ColorGreen
				userID := v.komes[i].UserID

				if v.komes[i].Is184Comment() {
					fg = termbox.ColorYellow
					userID = "184"
				}
				if i == v.ptr {
					fg = termbox.ColorDefault
				}

				for len(userID) < maxUserIDLen {
					userID += " "
				}
				for _, c := range userID {
					termbox.SetCell(x, y, c, fg, bg)
					x++
				}
			}

			termbox.SetCell(x, y, ' ', termbox.ColorDefault, bg)
			x++

			for _, c := range v.komes[i].Comment {
				termbox.SetCell(x, y, c, termbox.ColorDefault, bg)
				x += width(c)
			}
			for ;x < v.width; x++ {
				termbox.SetCell(x, y, ' ', termbox.ColorDefault, bg)
			}

			if i == v.ptr && !nowCmd {
				termbox.SetCursor(v.width - 1, y)
			}
			y++
		}
		for ; y < v.height - 2; y++ {
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
		right := fmt.Sprintf("%d%%", par)

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
		for _, c := range v.cmd {
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

func width(c rune) int {
	w := runewidth.RuneWidth(c)
	if w == 0 || w == 2 && runewidth.IsAmbiguousWidth(c) {
		w = 1
	}
	return w
}