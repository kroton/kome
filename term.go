package main

import (
	"github.com/nsf/termbox-go"
)

type TermIn struct {
	EventCh chan termbox.Event
	quitCh  chan struct{}
}

func NewTermIn() *TermIn {
	io := &TermIn {
		EventCh: make(chan termbox.Event),
		quitCh:  make(chan struct{}),
	}

	go io.process()
	return io
}

func (io *TermIn) process() {
	defer func(){
		recover()
		close(io.quitCh)
	}()

	for {
		io.EventCh <- termbox.PollEvent()
	}
}

func (io *TermIn) Close() {
	close(io.EventCh)
	<-io.quitCh
}