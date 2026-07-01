package model

import "time"

type Conversation struct {
	ID            string    `json:"conversation_id"`
	UserID        string    `json:"-"`
	Title         string    `json:"title"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	LastMessageAt time.Time `json:"last_message_at"`
}

type Message struct {
	ID              string    `json:"message_id"`
	ConversationID  string    `json:"conversation_id,omitempty"`
	Role            string    `json:"role"`
	Content         string    `json:"content"`
	TraceID         string    `json:"trace_id,omitempty"`
	ClientMessageID string    `json:"client_message_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}
