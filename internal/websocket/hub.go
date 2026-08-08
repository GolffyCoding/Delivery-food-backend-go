package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Message struct {
	Type      string      `json:"type"`
	Event     string      `json:"event"`
	Payload   interface{} `json:"payload"`
	Timestamp int64       `json:"timestamp"`
}

type Client struct {
	ID     string
	UserID uuid.UUID
	Role   string
	Send   chan []byte
}

type Hub struct {
	clients     map[string]*Client
	userClients map[uuid.UUID]map[string]struct{}
	roomClients map[string]map[string]struct{}
	logger      *zap.Logger
	mu          sync.RWMutex
}

func NewHub(logger *zap.Logger) *Hub {
	return &Hub{
		clients:     make(map[string]*Client),
		userClients: make(map[uuid.UUID]map[string]struct{}),
		roomClients: make(map[string]map[string]struct{}),
		logger:      logger,
	}
}

func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[client.ID] = client
	if h.userClients[client.UserID] == nil {
		h.userClients[client.UserID] = make(map[string]struct{})
	}
	h.userClients[client.UserID][client.ID] = struct{}{}
	h.logger.Info("websocket client connected", zap.String("client_id", client.ID), zap.String("user_id", client.UserID.String()))
}

func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client.ID]; !ok {
		return
	}
	delete(h.clients, client.ID)
	close(client.Send)

	if set, ok := h.userClients[client.UserID]; ok {
		delete(set, client.ID)
		if len(set) == 0 {
			delete(h.userClients, client.UserID)
		}
	}
	for room, set := range h.roomClients {
		delete(set, client.ID)
		if len(set) == 0 {
			delete(h.roomClients, room)
		}
	}
}

func (h *Hub) JoinRoom(clientID, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.roomClients[room] == nil {
		h.roomClients[room] = make(map[string]struct{})
	}
	h.roomClients[room][clientID] = struct{}{}
}

func (h *Hub) LeaveRoom(clientID, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.roomClients[room]; ok {
		delete(set, clientID)
		if len(set) == 0 {
			delete(h.roomClients, room)
		}
	}
}

func (h *Hub) SendToUser(userID uuid.UUID, msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for cid := range h.userClients[userID] {
		h.trySend(h.clients[cid], data)
	}
	return nil
}

func (h *Hub) SendToRoom(room string, msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for cid := range h.roomClients[room] {
		h.trySend(h.clients[cid], data)
	}
	return nil
}

func (h *Hub) SendToRole(role string, msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, client := range h.clients {
		if client.Role == role {
			h.trySend(client, data)
		}
	}
	return nil
}

func (h *Hub) trySend(client *Client, data []byte) {
	if client == nil {
		return
	}
	select {
	case client.Send <- data:
	default:
		// slow consumer, drop the message rather than block the hub
	}
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// PublishOrderEvent implements order.EventPublisher.
func (h *Hub) PublishOrderEvent(ctx context.Context, orderID uuid.UUID, eventType string, data interface{}) error {
	room := "order:" + orderID.String()
	return h.SendToRoom(room, Message{
		Type:      "order",
		Event:     eventType,
		Payload:   data,
		Timestamp: time.Now().Unix(),
	})
}

// PublishDriverLocation implements tracking.EventPublisher.
func (h *Hub) PublishDriverLocation(ctx context.Context, driverID uuid.UUID, data interface{}) error {
	room := "driver:" + driverID.String()
	return h.SendToRoom(room, Message{
		Type:      "driver",
		Event:     "driver.location",
		Payload:   data,
		Timestamp: time.Now().Unix(),
	})
}

// PublishDispatchAlert implements tracking.EventPublisher. Any client that joins the
// "dispatch:alerts" room (e.g. an admin/support dashboard) gets server-verified
// anomalies pushed live instead of having to take a driver's word for a GPS glitch.
func (h *Hub) PublishDispatchAlert(ctx context.Context, data interface{}) error {
	return h.SendToRoom("dispatch:alerts", Message{
		Type:      "dispatch",
		Event:     "dispatch.alert",
		Payload:   data,
		Timestamp: time.Now().Unix(),
	})
}

// PublishTicketEvent implements support.EventPublisher, pushing ticket lifecycle
// events to anyone watching the admin-only "support:queue" room in real time — new
// tickets appear on an agent's screen the instant they're filed instead of the agent
// having to refresh/poll a list.
func (h *Hub) PublishTicketEvent(ctx context.Context, eventType string, ticket interface{}) error {
	return h.SendToRoom("support:queue", Message{
		Type:      "support",
		Event:     eventType,
		Payload:   ticket,
		Timestamp: time.Now().Unix(),
	})
}

// SendToUserCtx implements notification.PushNotifier.
func (h *Hub) SendToUserCtx(ctx context.Context, userID uuid.UUID, payload interface{}) error {
	return h.SendToUser(userID, Message{
		Type:      "notification",
		Event:     "notification.new",
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	})
}

// NotifyDriver implements driver.EventNotifier.
func (h *Hub) NotifyDriver(ctx context.Context, driverID uuid.UUID, eventType string, data interface{}) error {
	return h.SendToUser(driverID, Message{
		Type:      "driver",
		Event:     eventType,
		Payload:   data,
		Timestamp: time.Now().Unix(),
	})
}
