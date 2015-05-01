package main

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

const (
	createUserTable = `create table if not exists user(id integer primary key, name varchar(255))`
)

type User struct {
	ID   int64  `xml:"id"`
	Name string `xml:"nickname"`
}

type UserRepo struct {
	db *sql.DB
	mp map[int64]User
}

func NewUserRepo(path string) (*UserRepo, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(createUserTable); err != nil {
		return nil, err
	}
	return &UserRepo{db: db, mp: make(map[int64]User)}, nil
}

func (r *UserRepo) Get(id int64) (User, error) {
	if user, ok := r.mp[id]; ok {
		return user, nil
	}

	user, err := r.read(id)
	if err == nil {
		r.mp[id] = user
	}
	return user, err
}

func (r *UserRepo) Save(user User) error {
	r.mp[user.ID] = user
	return r.write(user)
}

func (r *UserRepo) write(user User) error {
	_, err := r.db.Exec("insert into user values(?, ?)", user.ID, user.Name)
	return err
}

func (r *UserRepo) update(user User) error {
	_, err := r.db.Exec("update user set name = ? where id = ?", user.Name, user.ID)
	return err
}

func (r *UserRepo) read(id int64) (User, error) {
	row := r.db.QueryRow("select from user where id = ?", id)
	var user User
	err := row.Scan(&user.ID, &user.Name)
	return user, err
}
