package tenant

import (
	"time"

	"github.com/google/uuid"
)

// Message merepresentasikan pesan yang terkait dengan tenant tertentu
type Message struct {
    ID        uuid.UUID       `json:"id"`
    TenantID  uuid.UUID       `json:"tenant_id"`
    Payload   string 		  `json:"payload"`
    CreatedAt time.Time       `json:"created_at"`
}
type MessageListResponse struct {
	Data       []Message `json:"data"`
	NextCursor string    `json:"next_cursor,omitempty"`
}