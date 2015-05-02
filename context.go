package main

import (
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
)

const createUserTable = `create table if not exists user(id integer primary key, name varchar(255))`

var (
	nicoCookieName     = "user_session"
	nicoCookieValueReg = regexp.MustCompile(`^user_session_\d+_[0-9a-f]{64}$`)
	nicoGlobalURL, _   = url.Parse("http://nicovideo.jp")
)

type Context struct {
	Mail     string         `json:"mail"`
	Password string         `json:"password"`
	Session  string         `json:"session"`
	Client   http.Client    `json:"-"`
	db       *sql.DB        `json:"-"`
	mp       map[int64]User `json:"-"`
	dir      string         `json:"-"`
	confPath string         `json:"-"`
	dbPath   string         `json:"-"`
}

func LoadContext(dir string) (*Context, error) {
	ctx := new(Context)
	ctx.dir = dir
	ctx.confPath = dir + "/config.json"
	ctx.dbPath = dir + "/user.sqlite"

	f, err := os.Open(ctx.confPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file %v", ctx.confPath)
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(ctx); err != nil {
		return nil, fmt.Errorf("failed to parse config file %v", ctx.confPath)
	}

	if ctx.db, err = sql.Open("sqlite3", ctx.dbPath); err != nil {
		return nil, fmt.Errorf("failed to open user database %v", ctx.dbPath)
	}
	if _, err := ctx.db.Exec(createUserTable); err != nil {
		return nil, err
	}

	ctx.mp = make(map[int64]User)
	ctx.Client = clientWithSession(ctx.Session)
	return ctx, nil
}

func clientWithCookie() http.Client {
	jar, _ := cookiejar.New(nil)
	client := http.Client{Jar: jar}
	return client
}

func clientWithSession(session string) http.Client {
	client := clientWithCookie()
	client.Jar.SetCookies(nicoGlobalURL, []*http.Cookie{
		&http.Cookie{
			Domain: nicoGlobalURL.Host,
			Path:   "/",
			Name:   nicoCookieName,
			Value:  session,
			Secure: false,
		},
	})
	return client
}

func (ctx *Context) SaveConfig() error {
	b, err := json.MarshalIndent(ctx, "", "	")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(ctx.confPath, b, 0777)
}

func (ctx *Context) HeartBeat() bool {
	res, err := ctx.Client.Get("http://live.nicovideo.jp/api/heartbeat")
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

func (ctx *Context) Login() bool {
	client := clientWithCookie()
	_, err := client.PostForm(
		"https://secure.nicovideo.jp/secure/login?site=nicolive",
		url.Values{
			"mail":     {ctx.Mail},
			"password": {ctx.Password},
		},
	)
	if err != nil {
		return false
	}

	for _, cookie := range client.Jar.Cookies(nicoGlobalURL) {
		if cookie.Name == nicoCookieName && nicoCookieValueReg.MatchString(cookie.Value) {
			ctx.Session = cookie.Value
			ctx.Client = clientWithSession(ctx.Session)
			return true
		}
	}

	return false
}

func (ctx *Context) GetUser(id int64) (User, error) {
	if user, ok := ctx.mp[id]; ok {
		return user, nil
	}
	if user, err := ctx.readUser(id); err == nil {
		ctx.mp[id] = user
		return user, nil
	}

	u := fmt.Sprintf("http://seiga.nicovideo.jp/api/user/info?id=%d", id)
	res, err := ctx.Client.Get(u)
	if err != nil {
		return User{}, err
	}
	defer res.Body.Close()

	var resXML struct {
		XMLName xml.Name `xml:"response"`
		User    User     `xml:"user"`
	}
	if err := xml.NewDecoder(res.Body).Decode(&resXML); err != nil {
		return User{}, err
	}
	if resXML.User.ID != id || resXML.User.Name == "-" {
		return User{}, errors.New("failed to getting user info")
	}

	ctx.mp[id] = resXML.User
	ctx.writeUser(resXML.User)
	return resXML.User, nil
}

func (ctx *Context) writeUser(user User) error {
	_, err := ctx.db.Exec("insert into user values(?, ?)", user.ID, user.Name)
	return err
}

func (ctx *Context) readUser(id int64) (User, error) {
	row := ctx.db.QueryRow("select from user where id = ?", id)
	var user User
	err := row.Scan(&user.ID, &user.Name)
	return user, err
}
