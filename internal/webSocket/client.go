package websocket

import (
	"time"

	"github.com/gorilla/websocket"
)

const (
	WRITEWAIT      = 10 * time.Second
	PONGWAIT       = 60 * time.Second
	PINGPERIOD     = (PONGWAIT * 9) / 10
	maxMessageSize = 512
)

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

func NewClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(PONGWAIT))
	c.conn.SetPongHandler(func(appData string) error {
		c.conn.SetReadDeadline(time.Now().Add(PONGWAIT))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(PINGPERIOD)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(
				time.Now().Add(WRITEWAIT),
			)
			if !ok {
				c.conn.WriteMessage(
					websocket.CloseMessage, []byte{},
				)
				return
			}
			err := c.conn.WriteMessage(
				websocket.TextMessage, message,
			)
			if err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(
				time.Now().Add(WRITEWAIT),
			)
			err := c.conn.WriteMessage(
				websocket.PingMessage, nil,
			)
			if err != nil {
				return
			}
		}
	}
}
