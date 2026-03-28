package main

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type Cache struct {
	mu     sync.Mutex
	cnns   map[int64]*websocket.Conn
	newCnn chan int64
}

func CreateCache() *Cache {
	return &Cache{
		mu:     sync.Mutex{},
		cnns:   make(map[int64]*websocket.Conn),
		newCnn: make(chan int64),
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Request struct {
	Data       []byte `json:"data"`
	ReceiverId int64  `json:"receiver_id"`
}

type Response struct {
	Data     []byte `json:"data"`
	SenderId int64  `json:"sender_id"`
}

func (c *Cache) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	conn, err := upgrader.Upgrade(resp, req, nil)
	if err != nil {
		log.Println("Ошибка создания webSocket connection: ", err)
		return
	}
	c.mu.Lock()
	re := regexp.MustCompile(`[0-9]`)
	userIdStr := strings.Join(re.FindAllString(req.Header.Get("User-Id"), -1), "")
	userId, err := strconv.ParseInt(userIdStr, 10, 64)
	if err != nil {
		log.Println("Неверный userId: ", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Вы ввели неверный userId"))
		return
	}
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
