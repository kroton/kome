package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io/ioutil"
	"net"
	"regexp"
	"strconv"
	"sync"
	"time"
)

var (
	tagThreadBegin  = []byte("<thread ")
	tagThreadEnd    = append([]byte("/>"), 0)
	tagChatBegin    = []byte("<chat ")
	tagChatEnd      = append([]byte("</chat>"), 0)
	tagChatResBegin = []byte("<chat_result ")
	tagChatResEnd   = append([]byte("/>"), 0)
	rawUserIDReg    = regexp.MustCompile(`^\d+$`)
)

type User struct {
	ID        int64  `xml:"id"`
	Name      string `xml:"nickname"`
	IsRawUser bool   `xml:"-"`
}

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

	User struct {
		UserID    string `xml:"user_id"`
		Name      string `xml:"nickname"`
		IsPremium int    `xml:"is_premium"`
	} `xml:"user"`

	Ms struct {
		Addr   string `xml:"addr"`
		Port   int    `xml:"port"`
		Thread int64  `xml:"thread"`
	} `xml:"ms"`
}

type Thread struct {
	ResultCode int    `xml:"resultcode,attr"`
	LastRes    int    `xml:"last_res,attr"`
	Ticket     string `xml:"ticket,attr"`
	ServerTime int64  `xml:"server_time,attr"`
}

type Chat struct {
	XMLName xml.Name `xml:"chat"`
	Thread  int64    `xml:"thread,attr"`
	No      int      `xml:"no,attr"`
	Vpos    int64    `xml:"vpos,attr"`
	Date    int64    `xml:"date,attr"`
	UserID  string   `xml:"user_id,attr"`
	Premium int      `xml:"premium,attr"`
	Mail    string   `xml:"mail,attr"`
	Ticket  string   `xml:"ticket,attr"`
	PostKey string   `xml:"postkey,attr"`
	Comment string   `xml:",innerxml"`
	User    User     `xml:"-"`
}

type ChatResult struct {
	Status int `xml:"status"`
	No     int `xml:"no"`
}

type Live struct {
	account *Account
	repo    *UserRepo

	LiveID string
	Status PlayerStatus

	socket   *net.TCPConn
	buf      []byte
	acc      []byte
	thread   Thread
	openTime int64

	KomeCh chan Chat
	sig    chan struct{}
	quit   chan struct{}

	mu     sync.Mutex
	lastNo int
}

func NewLive(account *Account, repo *UserRepo, liveID string) *Live {
	return &Live{
		account: account,
		repo:    repo,
		LiveID:  liveID,
		buf:     make([]byte, 2048),
		acc:     make([]byte, 0, 2048),
		KomeCh:  make(chan Chat, 1024),
		sig:     make(chan struct{}, 1),
		quit:    make(chan struct{}, 1),
	}
}

func (lv *Live) GetPlayerStatus() error {
	u := fmt.Sprintf("http://watch.live.nicovideo.jp/api/getplayerstatus?v=%s", lv.LiveID)
	client := lv.account.NewClient()
	res, err := client.Get(u)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if err := xml.NewDecoder(res.Body).Decode(&lv.Status); err != nil {
		return err
	}
	if lv.Status.Status != "ok" {
		return errors.New("playerstatus should be ok")
	}

	return nil
}

func (lv *Live) write(b []byte) error {
	b = append(b, 0)
	for len(b) > 0 {
		n, err := lv.socket.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

func (lv *Live) Connect(timeout time.Duration) error {
	addr := fmt.Sprintf("%s:%d", lv.Status.Ms.Addr, lv.Status.Ms.Port)
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return err
	}
	lv.socket, err = net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return err
	}

	t := fmt.Sprintf(`<thread thread="%d" version="20061206" res_from="-1000"/>`, lv.Status.Ms.Thread)
	if err := lv.write([]byte(t)); err != nil {
		lv.socket.Close()
		return err
	}

	ch := make(chan error, 1)
	go func() {
		for {
			n, err := lv.socket.Read(lv.buf)
			if err != nil {
				ch <- err
				return
			}

			lv.acc = append(lv.acc, lv.buf[0:n]...)
			if bytes.HasPrefix(lv.acc, tagThreadBegin) {
				p := bytes.Index(lv.acc, tagThreadEnd)
				if p < 0 {
					continue
				}

				end := p + len(tagThreadEnd)
				if err := xml.Unmarshal(lv.acc[0:end], &lv.thread); err != nil {
					ch <- err
					return
				}

				lv.openTime = time.Now().Unix()
				lv.lastNo = lv.thread.LastRes
				lv.acc = lv.acc[end:]
				ch <- nil
				return
			}
		}
	}()

	select {
	case err := <-ch:
		if err != nil {
			lv.socket.Close()
			return err
		}
	case <-time.After(timeout):
		lv.socket.Close()
		<-ch
		return errors.New("timeout")
	}

	go lv.process()
	return nil
}

func (lv *Live) process() {
	defer func() {
		lv.quit <- struct{}{}
	}()

	for {
		for {
			if bytes.HasPrefix(lv.acc, tagChatBegin) {
				p := bytes.Index(lv.acc, tagChatEnd)
				if p < 0 {
					break
				}

				end := p + len(tagChatEnd)
				cut := lv.acc[0:end]
				lv.acc = lv.acc[end:]

				var kome Chat
				if err := xml.Unmarshal(cut, &kome); err != nil {
					continue
				}

				isRawUser := rawUserIDReg.MatchString(kome.UserID)
				if isRawUser {
					id, err := strconv.ParseInt(kome.UserID, 10, 64)
					if err == nil {
						kome.User, err = lv.repo.Get(id)
					}
					if err != nil {
						kome.User.ID = 0
						kome.User.Name = kome.UserID
					}
					kome.User.IsRawUser = true
				} else {
					kome.User.IsRawUser = false
					kome.User.ID = 0
					kome.User.Name = "184"
				}

				lv.mu.Lock()
				lv.lastNo = kome.No
				lv.mu.Unlock()

				select {
				case lv.KomeCh <- kome:
				case <-lv.sig:
					return
				}

				continue
			}

			if bytes.HasPrefix(lv.acc, tagChatResBegin) {
				p := bytes.Index(lv.acc, tagChatResEnd)
				if p < 0 {
					break
				}

				end := p + len(tagChatResEnd)
				cut := lv.acc[0:end]
				lv.acc = lv.acc[end:]

				var res ChatResult
				if err := xml.Unmarshal(cut, &res); err != nil {
					continue
				}

				lv.mu.Lock()
				lv.lastNo = res.No
				lv.mu.Unlock()

				continue
			}

			break
		}

		n, err := lv.socket.Read(lv.buf)
		if err != nil {
			return
		}

		lv.acc = append(lv.acc, lv.buf[0:n]...)
	}
}

func (lv *Live) Close() {
	lv.socket.Close()
	lv.sig <- struct{}{}
	<-lv.quit
}

func (lv *Live) getPostKey() (string, error) {
	lv.mu.Lock()
	blockNum := lv.lastNo / 10
	lv.mu.Unlock()

	u := fmt.Sprintf("http://live.nicovideo.jp/api/getpostkey?thread=%d&block_no=%d", lv.Status.Ms.Thread, blockNum)
	client := lv.account.NewClient()
	res, err := client.Get(u)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if len(b) < 8 {
		return "", errors.New("failed at getting post key")
	}

	return string(b[8:]), nil
}

func (lv *Live) calcVpos() int64 {
	return 100 * (lv.thread.ServerTime - lv.Status.Stream.StartTime + time.Now().Unix() - lv.openTime)
}

func (lv *Live) SendKome(comment string, is184 bool) error {
	postkey, err := lv.getPostKey()
	if err != nil {
		return err
	}

	vpos := lv.calcVpos()
	mail := ""
	if is184 {
		mail = "184"
	}

	kome := Chat{
		Thread:  lv.Status.Ms.Thread,
		Ticket:  lv.thread.Ticket,
		Vpos:    vpos,
		PostKey: postkey,
		UserID:  lv.Status.User.UserID,
		Premium: lv.Status.User.IsPremium,
		Mail:    mail,
		Comment: html.EscapeString(comment),
	}

	b, err := xml.Marshal(kome)
	if err != nil {
		return err
	}
	if err := lv.write(b); err != nil {
		return err
	}
	return nil
}
