package main

import (
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"net/http"
	"regexp"
	"strconv"
)

const createUserTable = `create table if not exists user(id integer primary key, name varchar(255))`

func OpenWithMigrate(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open user database %v", path)
	}
	if _, err := db.Exec(createUserTable); err != nil {
		return nil, err
	}
	return db, nil
}

var rawUserIDReg = regexp.MustCompile(`^\d+$`)

type UserRepo struct {
	db *sql.DB
	mp map[int64]User
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{
		db: db,
		mp: make(map[int64]User),
	}
}

func (r *UserRepo) Get(ID string) User {
	if !rawUserIDReg.MatchString(ID) {
		return User{
			IsRawUser: false,
			ID:        0,
			Name:      "184",
		}
	}

	id, err := strconv.ParseInt(ID, 10, 64)
	if err != nil {
		return User{
			IsRawUser: true,
			ID:        0,
			Name:      ID,
		}
	}

	u, err := r.getByRawID(id)
	if err != nil {
		return User{
			IsRawUser: true,
			ID:        id,
			Name:      ID,
		}
	}

	u.IsRawUser = true
	return u
}

func (r *UserRepo) writeToDB(user User) error {
	_, err := r.db.Exec("insert into user values(?, ?)", user.ID, user.Name)
	return err
}

func (r *UserRepo) readFromDB(id int64) (User, error) {
	row := r.db.QueryRow("select from user where id = ?", id)
	var user User
	err := row.Scan(&user.ID, &user.Name)
	return user, err
}

func (r *UserRepo) getByRawID(id int64) (User, error) {
	if user, ok := r.mp[id]; ok {
		return user, nil
	}
	if user, err := r.readFromDB(id); err == nil {
		r.mp[id] = user
		return user, nil
	}
	user, err := getUserFromAPI(id)
	if err == nil {
		r.mp[id] = user
		r.writeToDB(user)
		return user, nil
	}
	return User{}, err
}

func getUserFromAPI(id int64) (User, error) {
	u := fmt.Sprintf("http://api.ce.nicovideo.jp/api/v1/user.info?user_id=%d", id)
	res, err := http.Get(u)
	if err != nil {
		return User{}, err
	}
	defer res.Body.Close()

	var resXML struct {
		XMLName xml.Name `xml:"nicovideo_user_response"`
		Status  string   `xml:"status,attr"`
		User    User     `xml:"user"`
	}
	if err := xml.NewDecoder(res.Body).Decode(&resXML); err != nil {
		return User{}, err
	}
	if resXML.Status != "ok" {
		return User{}, errors.New("user not found")
	}
	return resXML.User, nil
}
