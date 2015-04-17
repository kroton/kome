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
	recv_buf      = 2048
	kome_chan_buf = 256
)

var (
	getPlayerStatusAPIURL = "http://watch.live.nicovideo.jp/api/getplayerstatus?v=%s"
	errPlayerStatusFail   = errors.New("playerstatus should be ok")

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

type Live struct {
	client http.Client
	socket *net.TCPConn

	LiveID string
	Status PlayerStatus
	Thread KomeThread
}

func NewLive(client http.Client, liveID string) *Live {
	return &Live {
		client: client,
		LiveID: liveID,
	}
}

func (lv *Live) getPlayerStatus() error {
	APIURL := fmt.Sprintf(getPlayerStatusAPIURL, lv.LiveID)

	res, err := lv.client.Get(APIURL)
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

func (lv *Live) Connect() (<-chan Kome, error) {
	addr := fmt.Sprintf("%s:%d", lv.Status.Ms.Addr, lv.Status.Ms.Port)

	tcp_addr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	if lv.socket, err = net.DialTCP("tcp", nil, tcp_addr); err != nil {
		return nil, err
	}

	data := fmt.Sprintf(`<thread thread="%d" version="20061206" res_from="-1000"/>`, lv.Status.Ms.Thread)
	b := []byte(data)

	// send thread data
	lv.socket.Write(append(b, 0))

	var acc []byte
	buf := make([]byte, recv_buf)

	for {
		n, err := lv.socket.Read(buf)
		if err != nil {
			lv.socket.Close()
			return nil, err
		}

		acc = append(acc, buf[0:n]...)

		if bytes.HasPrefix(acc, threadSt) {
			p := bytes.Index(acc, threadEd)
			if p < 0 {
				continue
			}

			end := p + len(threadEd)
			if err := xml.Unmarshal(acc[0:end], &lv.Thread); err != nil {
				lv.socket.Close()
				return nil, err
			}

			acc = acc[end:]
			break
		}
	}

	ch := make(chan Kome, kome_chan_buf)
	go lv.loop(buf, acc, ch)

	return ch, nil
}

func (lv *Live) loop(buf, acc []byte, ch chan<- Kome) {
	defer lv.socket.Close()

	for {
		n, err := lv.socket.Read(buf)
		if err != nil {
			return
		}

		acc = append(acc, buf[0:n]...)
		for {
			p := bytes.Index(acc, chatEd)
			if p < 0 {
				break
			}

			end := p + len(chatEd)
			mid := acc[0:end]

			acc = acc[end:]

			var kome Kome
			if err := xml.Unmarshal(mid, &kome); err != nil {
				continue
			}

			next := func() (res bool) {
				defer func(){
					if err := recover(); err != nil {
						res = false
					}
				}()

				res = true
				ch <- kome
				return
			}()
			if !next {
				return
			}
		}
	}
}

func (lv *Live) Close() {
	lv.socket.Close()
}