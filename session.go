package main

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

var (
	nicoLoginURL     = "https://secure.nicovideo.jp/secure/login?site=nicolive"
	nicoCookieURL, _ = url.Parse("https://secure.nicovideo.jp")
)

func clientWithCookie() http.Client {
	jar, _ := cookiejar.New(nil)
	client := http.Client {Jar: jar}
	return client
}

func directLogin(mail, password string) (http.Client, bool) {
	client := clientWithCookie()
	client.PostForm(
		nicoLoginURL,
		url.Values {
			"mail":     { mail },
			"password": { password },
		},
	)

	var ok bool
	for _, cookie := range client.Jar.Cookies(nicoCookieURL) {
		if cookie.Name == "user_session" && strings.HasPrefix(cookie.Value, "user_session_") {
			ok = true
			break
		}
	}
	return client, ok
}