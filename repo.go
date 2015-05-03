package main

import (
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"net/http"
)

const createUserTable = `create table if not exists user(id integer primary key, name varchar(255))`

type UserRepo struct {
	db *sql.DB
	mp map[int64]User
}

func LoadUserRepo(path string) (*UserRepo, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open user database %v", path)
	}
	if _, err := db.Exec(createUserTable); err != nil {
		return nil, err
	}
	return &UserRepo{db: db, mp: make(map[int64]User)}, nil
}

func (r *UserRepo) write(user User) error {
	_, err := r.db.Exec("insert into user values(?, ?)", user.ID, user.Name)
	return err
}

func (r *UserRepo) read(id int64) (User, error) {
	row := r.db.QueryRow("select from user where id = ?", id)
	var user User
	err := row.Scan(&user.ID, &user.Name)
	return user, err
}

func (r *UserRepo) Get(id int64) (User, error) {
	if user, ok := r.mp[id]; ok {
		return user, nil
	}
	if user, err := r.read(id); err == nil {
		r.mp[id] = user
		return user, nil
	}
	user, err := getUserFromAPI(id)
	if err == nil {
		r.mp[id] = user
		r.write(user)
		return user, nil
	}
	return User{}, err
}

func getUserFromAPI(id int64) (User, error) {
	u := fmt.Sprintf("http://seiga.nicovideo.jp/api/user/info?id=%d", id)
	res, err := http.Get(u)
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
		return User{}, errors.New("user not found")
	}
	return resXML.User, nil
}
