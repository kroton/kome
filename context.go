package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
)

type Context struct {
	account *Account
	repo    *UserRepo
}

func (ctx *Context) NewClient() http.Client {
	return ctx.account.NewClient()
}

func (ctx *Context) GetUser(id int64) (User, error) {
	if user, err := ctx.repo.Get(id); err == nil {
		return user, nil
	}
	user, err := ctx.getUserFromAPI(id)
	if err == nil {
		ctx.repo.Save(user)
	}
	return user, err
}

func (ctx *Context) getUserFromAPI(id int64) (User, error) {
	u := fmt.Sprintf("http://seiga.nicovideo.jp/api/user/info?id=%d", id)
	client := ctx.account.NewClient()
	res, err := client.Get(u)
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
	return resXML.User, nil
}
