package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/gofrs/uuid"
)

var (
	// 新用户到来，通过该 channel 进行登记
	enteringChan = make(chan *User)
	// 用户离开，通过该 channel 进行登记
	leavingChan = make(chan *User)
	// 普通用户消息，通过该 channel 进行广播
	messageChan = make(chan Message, 8)
)

type User struct {
	ID      string
	Addr    string
	EnterAt time.Time
	MsgChan chan string
}

type Message struct {
	OwnerID string
	Content string
}

func main() {
	lis, err := net.Listen("tcp", ":4040")
	if err != nil {
		panic(err)
	}

	go broadcast()

	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go handleConn(conn)
	}
}

// 记录聊天室用户，并进行消息广播
// 1、新用户进入 2、用户普通消息 3、用户离开
func broadcast() {
	users := make(map[*User]struct{})
	for {
		select {
		case user := <-enteringChan:
			// 新用户进入
			users[user] = struct{}{}
		case user := <-leavingChan:
			// 有用户离开
			delete(users, user)
			// 避免协程泄露
			close(user.MsgChan)
		case msg := <-messageChan:
			// 给所有在线用户发送消息
			for user := range users {
				if user.ID == msg.OwnerID {
					continue
				}
				user.MsgChan <- msg.Content
			}
		}
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	// 1、新用户进来，构建用户实例
	user := &User{
		ID:      genUserID(),
		Addr:    conn.RemoteAddr().String(),
		EnterAt: time.Now(),
		MsgChan: make(chan string, 8),
	}

	// 2、新开一个协程给用户发送消息
	go sendMessage(conn, user.MsgChan)

	// 3、给当前用户发送欢迎消息，向所有用户告知新用户进入
	user.MsgChan <- "Welcome " + user.ID + " to chat room!"
	messageChan <- Message{OwnerID: user.ID, Content: user.ID + " has entered the chat room!"}

	// 4、将新用户登记到全局的用户列表中
	enteringChan <- user

	// 踢出超时用户
	var userActive = make(chan struct{})
	go func() {
		d := 5 * time.Minute
		timer := time.NewTimer(d)
		for {
			select {
			case <-userActive:
				timer.Reset(d)
			case <-timer.C:
				conn.Close()
			}
		}
	}()

	// 5、循环读取用户发送的消息
	input := bufio.NewScanner(conn)
	for input.Scan() {
		messageChan <- Message{OwnerID: user.ID, Content: user.ID + ": " + input.Text()}
		// 用户处于活跃状态
		userActive <- struct{}{}
	}

	if err := input.Err(); err != nil {
		log.Println("读取错误：", err)
	}

	// 6、用户离开，从全局用户列表中删除
	leavingChan <- user
	messageChan <- Message{OwnerID: user.ID, Content: user.ID + " has left the chat room!"}
}

func genUserID() string {
	id, _ := uuid.NewV4()
	return id.String()
}

func sendMessage(conn net.Conn, msgChan <-chan string) {
	for msg := range msgChan {
		fmt.Fprintln(conn, msg)
	}
}
