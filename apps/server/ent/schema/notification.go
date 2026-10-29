package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Notification is an alert event waiting to be sent. It's written in the same
// transaction as the state change that caused it, and any worker may send it,
// so a worker dying can't lose an alert.
type Notification struct {
	ent.Schema
}

func (Notification) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int("endpoint_id"),
		field.String("alert_type"),
		field.JSON("event", json.RawMessage{}),
		field.Int("attempts").Default(0),
		field.Time("next_attempt_at").Default(time.Now),
		field.String("last_error").Default(""),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Notification) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("endpoint", Endpoint.Type).Ref("notifications").Field("endpoint_id").Unique().Required(),
	}
}

func (Notification) Indexes() []ent.Index {
	return []ent.Index{
		// Events for one endpoint and provider are sent in id order.
		index.Fields("endpoint_id", "alert_type", "id"),
	}
}
