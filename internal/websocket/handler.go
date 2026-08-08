package websocket

import (
	"encoding/json"
	"fmt"
	"time"

	fastwebsocket "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/opendelivery/opendelivery/internal/auth"
	"github.com/opendelivery/opendelivery/pkg/middleware"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

// restrictedRooms maps room names that carry sensitive operational data to the roles
// allowed to join them. "dispatch:alerts" carries server-verified GPS anomaly reports
// tied to specific drivers/orders, so it's restricted to admin/support staff rather
// than joinable by any authenticated client the way order/driver tracking rooms are.
var restrictedRooms = map[string]string{
	"dispatch:alerts": string(auth.RoleAdmin),
	"support:queue":   string(auth.RoleAdmin),
}

type Handler struct {
	hub      *Hub
	logger   *zap.Logger
	upgrader fastwebsocket.FastHTTPUpgrader
}

func NewHandler(hub *Hub, logger *zap.Logger) *Handler {
	return &Handler{
		hub:    hub,
		logger: logger,
		upgrader: fastwebsocket.FastHTTPUpgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(ctx *fasthttp.RequestCtx) bool { return true },
		},
	}
}

type controlMessage struct {
	Action string `json:"action"` // "join" or "leave"
	Room   string `json:"room"`
}

// Serve upgrades the connection and pumps messages between the client and the hub.
// Auth is enforced beforehand via AuthMiddleware.RequireAuth() on the route.
func (h *Handler) Serve(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid user")
	}
	role := middleware.Role(c)

	err = h.upgrader.Upgrade(c.RequestCtx(), func(conn *fastwebsocket.Conn) {
		client := &Client{
			ID:     fmt.Sprintf("%s-%d", userID, time.Now().UnixNano()),
			UserID: userID,
			Role:   role,
			Send:   make(chan []byte, 32),
		}
		h.hub.Register(client)
		defer h.hub.Unregister(client)

		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					return
				}
				var ctrl controlMessage
				if json.Unmarshal(msg, &ctrl) == nil {
					switch ctrl.Action {
					case "join":
						if requiredRole, restricted := restrictedRooms[ctrl.Room]; restricted && client.Role != requiredRole {
							h.logger.Warn("rejected room join: insufficient role",
								zap.String("user_id", client.UserID.String()), zap.String("role", client.Role), zap.String("room", ctrl.Room))
							continue
						}
						h.hub.JoinRoom(client.ID, ctrl.Room)
					case "leave":
						h.hub.LeaveRoom(client.ID, ctrl.Room)
					}
				}
			}
		}()

		for {
			select {
			case data, ok := <-client.Send:
				if !ok {
					return
				}
				if err := conn.WriteMessage(fastwebsocket.TextMessage, data); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	})

	if err != nil {
		h.logger.Warn("websocket upgrade failed", zap.Error(err))
		return fiber.NewError(fiber.StatusBadRequest, "websocket upgrade failed")
	}
	return nil
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	router.Get("/ws", authMW.RequireAuth(), h.Serve)
}
