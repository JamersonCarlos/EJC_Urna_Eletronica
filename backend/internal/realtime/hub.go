// Package realtime mantém as conexões WebSocket por eleição:
// vários painéis de mesário (admins) e uma urna (celular).
package realtime

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUrna  Role = "urna"
)

type Message struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 25 * time.Second
	maxMsgSize = 4096
)

type Client struct {
	hub        *Hub
	conn       *websocket.Conn
	send       chan []byte
	ElectionID uint64
	Role       Role
	closeOnce  sync.Once
}

type room struct {
	admins map[*Client]struct{}
	urna   *Client
	// último estado de digitação para quem conectar no meio do voto
	lastTyping json.RawMessage
}

type Hub struct {
	mu    sync.RWMutex
	rooms map[uint64]*room
	// OnUrnaMessage é chamado para cada mensagem recebida da urna.
	OnUrnaMessage func(c *Client, msgType string, payload json.RawMessage)
}

func NewHub() *Hub {
	return &Hub{rooms: map[uint64]*room{}}
}

func (h *Hub) roomFor(id uint64) *room {
	r, ok := h.rooms[id]
	if !ok {
		r = &room{admins: map[*Client]struct{}{}}
		h.rooms[id] = r
	}
	return r
}

// Register adiciona a conexão e inicia as goroutines de leitura/escrita.
func (h *Hub) Register(conn *websocket.Conn, electionID uint64, role Role) *Client {
	c := &Client{hub: h, conn: conn, send: make(chan []byte, 64), ElectionID: electionID, Role: role}

	h.mu.Lock()
	r := h.roomFor(electionID)
	var replaced *Client
	if role == RoleUrna {
		replaced = r.urna
		r.urna = c
	} else {
		r.admins[c] = struct{}{}
	}
	typing := r.lastTyping
	h.mu.Unlock()

	if replaced != nil {
		replaced.Send(Message{Type: "replaced"})
		replaced.close()
	}

	go c.writePump()
	go c.readPump()

	if role == RoleUrna {
		h.BroadcastAdmins(electionID, Message{Type: "urna_status", Payload: map[string]bool{"online": true}})
	} else {
		c.Send(Message{Type: "urna_status", Payload: map[string]bool{"online": h.UrnaOnline(electionID)}})
		if len(typing) > 0 {
			c.Send(Message{Type: "urna_typing", Payload: typing})
		}
	}
	return c
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	r, ok := h.rooms[c.ElectionID]
	wasUrna := false
	if ok {
		if c.Role == RoleUrna && r.urna == c {
			r.urna = nil
			wasUrna = true
		} else {
			delete(r.admins, c)
		}
		if r.urna == nil && len(r.admins) == 0 {
			delete(h.rooms, c.ElectionID)
		}
	}
	h.mu.Unlock()
	if wasUrna {
		h.BroadcastAdmins(c.ElectionID, Message{Type: "urna_status", Payload: map[string]bool{"online": false}})
	}
}

func (h *Hub) SetTyping(electionID uint64, payload json.RawMessage) {
	h.mu.Lock()
	h.roomFor(electionID).lastTyping = payload
	h.mu.Unlock()
}

func (h *Hub) ClearTyping(electionID uint64) {
	h.mu.Lock()
	if r, ok := h.rooms[electionID]; ok {
		r.lastTyping = nil
	}
	h.mu.Unlock()
}

func (h *Hub) UrnaOnline(electionID uint64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	r, ok := h.rooms[electionID]
	return ok && r.urna != nil
}

func (h *Hub) BroadcastAdmins(electionID uint64, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if r, ok := h.rooms[electionID]; ok {
		for c := range r.admins {
			c.sendRaw(data)
		}
	}
}

func (h *Hub) SendUrna(electionID uint64, msg Message) {
	h.mu.RLock()
	var u *Client
	if r, ok := h.rooms[electionID]; ok {
		u = r.urna
	}
	h.mu.RUnlock()
	if u != nil {
		u.Send(msg)
	}
}

func (c *Client) Send(msg Message) {
	data, err := json.Marshal(msg)
	if err == nil {
		c.sendRaw(data)
	}
}

func (c *Client) sendRaw(data []byte) {
	defer func() { _ = recover() }() // canal pode ter sido fechado
	select {
	case c.send <- data:
	default:
		log.Printf("ws: buffer cheio, descartando cliente %s da eleição %d", c.Role, c.ElectionID)
		go c.close()
	}
}

func (c *Client) close() {
	c.closeOnce.Do(func() {
		c.hub.unregister(c)
		close(c.send)
		_ = c.conn.Close()
	})
}

func (c *Client) readPump() {
	defer c.close()
	c.conn.SetReadLimit(maxMsgSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var in struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(data, &in) != nil {
			continue
		}
		if in.Type == "ping" {
			c.Send(Message{Type: "pong"})
			continue
		}
		if c.Role == RoleUrna && c.hub.OnUrnaMessage != nil {
			c.hub.OnUrnaMessage(c, in.Type, in.Payload)
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()
	for {
		select {
		case data, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
