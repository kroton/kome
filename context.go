package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
)

var (
	nicoCookieName     = "user_session"
	nicoCookieValueReg = regexp.MustCompile(`^user_session_\d+_[0-9a-f]{64}$`)
	nicoGlobalURL, _   = url.Parse("http://nicovideo.jp")
)

type NicoContext struct {
	Mail     string `json:"mail"`
	Password string `json:"password"`
	Session  string `json:"session"`
}

func LoadContext(fileName string) (*NicoContext, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to open context file %v", fileName)
	}
	defer f.Close()

	c := new(NicoContext)
	if err := json.NewDecoder(f).Decode(c); err != nil {
		return nil, fmt.Errorf("failed to parse context file %v", fileName)
	}
	return c, nil
}

func (nc *NicoContext) SaveTo(fileName string) error {
	b, err := json.MarshalIndent(nc, "", "	")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(fileName, b, 0777)
}

func (nc *NicoContext) NewClient() http.Client {
	return nc.client()
}

func (nc *NicoContext) client() http.Client {
	client := clientWithCookie()
	client.Jar.SetCookies(nicoGlobalURL, []*http.Cookie{
		&http.Cookie{
			Domain: nicoGlobalURL.Host,
			Path:   "/",
			Name:   nicoCookieName,
			Value:  nc.Session,
			Secure: false,
		},
	})
	return client
}

func (nc *NicoContext) HeartBeat() bool {
	client := nc.client()
	res, err := client.Get("http://live.nicovideo.jp/api/heartbeat")
	if err != nil {
		return false
	}
	defer res.Body.Close()

	var h struct {
		Err struct {
			Code string `xml:"code"`
		} `xml:"error"`
	}
	if err := xml.NewDecoder(res.Body).Decode(&h); err != nil {
		return false
	}

	return h.Err.Code != "NOTLOGIN"
}

func (nc *NicoContext) Login() bool {
	client := clientWithCookie()
	_, err := client.PostForm(
		"https://secure.nicovideo.jp/secure/login?site=nicolive",
		url.Values{
			"mail":     {nc.Mail},
			"password": {nc.Password},
		},
	)
	if err != nil {
		return false
	}

	for _, cookie := range client.Jar.Cookies(nicoGlobalURL) {
		if cookie.Name == nicoCookieName && nicoCookieValueReg.MatchString(cookie.Value) {
			nc.Session = cookie.Value
			return true
		}
	}

	return false
}

func clientWithCookie() http.Client {
	jar, _ := cookiejar.New(nil)
	client := http.Client{Jar: jar}
	return client
}
