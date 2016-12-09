package main

import (
	"encoding/json"
	"encoding/xml"
	"errors"
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

type Account struct {
	Mail     string `json:"mail"`
	Password string `json:"password"`
	Session  string `json:"session"`
}

func LoadAccount(path string) (*Account, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open account file %v", path)
	}
	defer f.Close()

	a := new(Account)
	if err := json.NewDecoder(f).Decode(a); err != nil {
		return nil, fmt.Errorf("failed to parse account file %v", path)
	}
	return a, nil
}

func (a *Account) SaveTo(path string) error {
	b, err := json.MarshalIndent(a, "", "	")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, b, 0600)
}

func clientWithCookie() http.Client {
	jar, _ := cookiejar.New(nil)
	client := http.Client{Jar: jar}
	return client
}

func (a *Account) NewClient() http.Client {
	client := clientWithCookie()
	client.Jar.SetCookies(nicoGlobalURL, []*http.Cookie{
		&http.Cookie{
			Domain: nicoGlobalURL.Host,
			Path:   "/",
			Name:   nicoCookieName,
			Value:  a.Session,
			Secure: false,
		},
	})
	return client
}

func (a *Account) HeartBeat() error {
	client := a.NewClient()
	res, err := client.Get("http://live.nicovideo.jp/api/heartbeat")
	if err != nil {
		return err
	}
	defer res.Body.Close()

	var h struct {
		Err struct {
			Code string `xml:"code"`
		} `xml:"error"`
	}
	if err := xml.NewDecoder(res.Body).Decode(&h); err != nil {
		return err
	}
	if h.Err.Code == "NOTLOGIN" {
		return errors.New("not login")
	}

	return nil
}

func (a *Account) Login() error {
	client := clientWithCookie()
	_, err := client.PostForm(
		"https://secure.nicovideo.jp/secure/login?site=nicolive",
		url.Values{
			"mail":     {a.Mail},
			"password": {a.Password},
		},
	)
	if err != nil {
		return err
	}

	for _, cookie := range client.Jar.Cookies(nicoGlobalURL) {
		if cookie.Name == nicoCookieName && nicoCookieValueReg.MatchString(cookie.Value) {
			a.Session = cookie.Value
			return nil
		}
	}
	return errors.New("failed to login")
}
