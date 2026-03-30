package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type Cache struct {
	mu     sync.Mutex
	cnns   map[string]*websocket.Conn
	newCnn chan string
}

func CreateCache() *Cache {
	return &Cache{
		mu:     sync.Mutex{},
		cnns:   make(map[string]*websocket.Conn),
		newCnn: make(chan string),
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Request struct {
	Data       []byte `json:"data"`
	ReceiverId string `json:"receiver_id"`
}

type Response struct {
	Data     []byte `json:"data"`
	SenderId string `json:"sender_id"`
}

func (c *Cache) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	conn, err := upgrader.Upgrade(resp, req, nil)
	if err != nil {
		log.Println("Ошибка создания webSocket connection: ", err)
		return
	}
	c.mu.Lock()
	userId := req.Header.Get("User-Id")
	c.cnns[userId] = conn
	go func() {
		c.newCnn <- userId
	}()
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.cnns, userId)
		c.mu.Unlock()
		conn.Close()
	}()

	for {
		var (
			data Request
			cn   *websocket.Conn
			ok   bool
		)
		if err := conn.ReadJSON(&data); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			} else {
				log.Println("Ошибка чтения json: ", err)
				conn.WriteMessage(websocket.TextMessage, []byte("Ошибка чтения json message"))
				return
			}
		}
		if cn, ok = c.cnns[data.ReceiverId]; !ok || cn == nil {
			for {
				receiverId := <-c.newCnn
				if receiverId == data.ReceiverId {
					cn = c.cnns[receiverId]
					break
				} else {
					continue
				}
			}
		}
		if err := cn.WriteJSON(
			Response{
				Data:     data.Data,
				SenderId: userId,
			},
		); err != nil {
			log.Println("Ошибка отправки json message: ", err)
			conn.WriteMessage(websocket.TextMessage, []byte("Ошибка отправки json message"))
			continue
		}
	}
}

func main() {
	cache := CreateCache()
	log.Println("Сервер запускается")
	if err := http.ListenAndServe("0.0.0.0:8080", cache); err != nil {
		panic(fmt.Sprint("Ошибка запуска http сервера: ", err))
	}
}
