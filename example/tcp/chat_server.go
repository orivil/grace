// +build ignore

package main

import (
	"gopkg.in/orivil/grace.v1"
	"net"
	"fmt"
	"io"
	"gopkg.in/orivil/log.v0"
	"sync"
	"os"
)

var (
	pid = os.Getpid()

	logf = func(format string, args... interface{}) {

		log.Printf(fmt.Sprintf("[process: %d] ", pid) + format, args...)
	}

	sprintf = func(format string, args... interface{}) string {

		args = append([]interface{}{pid}, args...)

		return fmt.Sprintf("[process: %d] " + format, args...)
	}
)

var room = &Room{Users: make(map[User]net.Conn, 10)}

type Room struct {

	Users map[User]net.Conn
	sync.RWMutex
}

func (r *Room) AddUser(u User, c net.Conn) {
	r.Lock()
	r.Users[u] = c
	r.Unlock()
}

func (r *Room) RemoveUser(u User) {
	r.Lock()
	delete(r.Users, u)
	r.Unlock()
}

type User string

func (u User) String() string {
	return string(u)
}

type Msg  string

func (m Msg) String() string {
	return string(m)
}

func main() {

	grace.ListenSignal()

	addr := ":8081"
	err := grace.ListenNetAndServe("tcp", addr, func(c net.Conn) {

		var user User
		var name = make([]byte, 256)

		_, err := c.Read(name)
		if err != nil {
			logf("read user got error: %v\n", err)
			return
		}
		user = User(name)

		defer func() {
			room.RemoveUser(user)
			msg := sprintf("user %s left, goodbye!\n", user)
			broadcast(msg)
			broadRoom()

			logf("user %s left the room\n", user)
		}()

		logf("user %s joined the room\n", user)

		msg := sprintf("user %s jion, welcome!\n", user)
		broadcast(msg)

		room.AddUser(user, c)

		broadRoom()

		for {

			data := make([]byte, 1024)
			_, err := c.Read(data)
			// close current connect.
			if err != nil {
				return
			}

			msg := sprintf("[%s]:%s\n", user, string(data))
			broadcast(msg)
		}
	})

	if err != nil {
		log.Println(err)
	}
}

func broadcast(msg string) {

	room.RLock()
	for _, conn := range room.Users {

		io.WriteString(conn, msg)
	}
	room.RUnlock()
}

func broadRoom() {

	users := make([]string, len(room.Users))
	idx := 0
	room.RLock()
	for u := range room.Users {

		users[idx] = string(u)
		idx++
	}
	room.RUnlock()

	msg := sprintf("room users:%v\n", users)
	broadcast(msg)
}
