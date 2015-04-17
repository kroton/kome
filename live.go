package main

import (
	"net"
	"net/http"
	"fmt"
	"encoding/xml"
	"errors"
	"bytes"
)

const (
	recv_buf_size      = 2048
	kome_chan_buf_size = 256

	// Format
	getPlayerStatusAPI = "http://watch.live.nicovideo.jp/api/getplayerstatus?v=%s"
	firstThreadFormat  = `<thread thread="%d" version="20061206" res_from="-1000"/>`
)

var (
	errPlayerStatusFail = errors.New("playerstatus should be ok")

	threadSt = []byte("<thread")
	threadEd = append([]byte("/>"), 0)
	chatEd   = append([]byte("</chat>"), 0)
)

type PlayerStatus struct {
	Status string `xml:"status,attr"`

	Stream struct {
		Title       string `xml:"title"`
		Description string `xml:"description"`
		Community   string `xml:"default_community"`
		OwnerID     int64  `xml:"owner_id"`
		OwnerName   string `xml:"owner_name"`
		StartTime   int64  `xml:"start_time"`
		EndTime     int64  `xml:"end_time"`
	} `xml:"stream"`

	Ms struct{
		Addr   string `xml:"addr"`
		Port   int    `xml:"port"`
		Thread int64  `xml:"thread"`
	} `xml:"ms"`
}

type KomeThread struct {
	ResultCode int    `xml:"resultcode,attr"`
	LastRes    int    `xml:"last_res,attr"`
	Ticket     string `xml:"ticket,attr"`
	ServerTime int64  `xml:"server_time,attr"`
}

type Kome struct {
	No       int    `xml:"no,attr"`
	Vpos     int64  `xml:"vpos,attr"`
	Date     int64  `xml:"date,attr"`
	UserID   string `xml:"user_id,attr"`
	Premium  int    `xml:"premium,attr"`
	Mail     string `xml:"mail,attr"`
	Comment  string `xml:",innerxml"`
}

type NicoLive struct {
	quitCh chan struct{}
	KomeCh chan Kome

	buf []byte
	acc []byte

	client http.Client
	socket *net.TCPConn

	LiveID string
	Status PlayerStatus
	Thread KomeThread
}

func NewNicoLive(client http.Client, liveID string) *NicoLive {
	return &NicoLive {
		quitCh: make(chan struct{}),
		KomeCh: make(chan Kome, kome_chan_buf_size),
		buf:    make([]byte, recv_buf_size),
		acc:    make([]byte, 0, recv_buf_size),
		client: client,
		LiveID: liveID,
	}
}

func (lv *NicoLive) GetPlayerStatus() error {
	u := fmt.Sprintf(getPlayerStatusAPI, lv.LiveID)

	res, err := lv.client.Get(u)
	if err != nil {
		return err
	}

	defer res.Body.Close()
	if err := xml.NewDecoder(res.Body).Decode(&lv.Status); err != nil {
		return err
	}
	if lv.Status.Status != "ok" {
		return errPlayerStatusFail
	}

	return nil
}

func (lv *NicoLive) Connect() error {
	addr := fmt.Sprintf("%s:%d", lv.Status.Ms.Addr, lv.Status.Ms.Port)

	tcp_addr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return err
	}

	lv.socket, err = net.DialTCP("tcp", nil, tcp_addr)
	if err != nil {
		return err
	}

	d := fmt.Sprintf(firstThreadFormat, lv.Status.Ms.Thread)
	b := append([]byte(d), 0)
	lv.socket.Write(b)

	for {
		n, err := lv.socket.Read(lv.buf)
		if err != nil {
			lv.socket.Close()
			return err
		}

		lv.acc = append(lv.acc, lv.buf[0:n]...)
		if bytes.HasPrefix(lv.acc, threadSt) {
			p := bytes.Index(lv.acc, threadEd)
			if p < 0 {
				continue
			}

			end := p + len(threadEd)
			if err := xml.Unmarshal(lv.acc[0:end], &lv.Thread); err != nil {
				lv.socket.Close()
				return err
			}

			lv.acc = lv.acc[end:]
			break
		}
	}

	go lv.process()
	return nil
}

func (lv *NicoLive) process() {
	defer func(){
		lv.socket.Close()
		close(lv.KomeCh)
		close(lv.quitCh)
	}()

	for {
		n, err := lv.socket.Read(lv.buf)
		if err != nil {
			return
		}

		lv.acc = append(lv.acc, lv.buf[0:n]...)
		for {
			p := bytes.Index(lv.acc, chatEd)
			if p < 0 {
				break
			}

			end := p + len(chatEd)
			cut := lv.acc[0:end]
			lv.acc = lv.acc[end:]

			var kome Kome
			if err := xml.Unmarshal(cut, &kome); err != nil {
				continue
			}

			lv.KomeCh <- kome
		}
	}
}

func (lv *NicoLive) Close() {
	lv.socket.Close()
	<-lv.quitCh
}