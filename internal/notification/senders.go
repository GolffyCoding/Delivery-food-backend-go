package notification

import (
	"context"

	"github.com/google/uuid"
)

// PushNotifier abstracts the transport used to push in-app/websocket notifications
// (implemented by the websocket Hub in this project).
type PushNotifier interface {
	SendToUserCtx(ctx context.Context, userID uuid.UUID, payload interface{}) error
}

// InAppSender delivers push/in_app notifications over the websocket hub. The
// notification row itself is always persisted by the service regardless of
// whether a live connection is present, so it is safe to "miss" delivery here.
type InAppSender struct {
	pusher PushNotifier
}

func NewInAppSender(pusher PushNotifier) *InAppSender {
	return &InAppSender{pusher: pusher}
}

func (s *InAppSender) CanHandle(t Type) bool { return t == TypePush || t == TypeInApp }

func (s *InAppSender) Send(ctx context.Context, n *Notification) error {
	if s.pusher == nil {
		return nil
	}
	return s.pusher.SendToUserCtx(ctx, n.UserID, n)
}

// LogSender is a stand-in for email/SMS providers: in this environment there is no
// real SMTP/SMS gateway configured, so it records the send as successful without
// making an outbound call. Swap in a real provider-backed Sender to go live.
type LogSender struct {
	handles Type
}

func NewLogSender(t Type) *LogSender { return &LogSender{handles: t} }

func (s *LogSender) CanHandle(t Type) bool { return t == s.handles }

func (s *LogSender) Send(ctx context.Context, n *Notification) error {
	return nil
}
