package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Worker is a live checker replica. Each worker heartbeats this row, and every
// worker hashes endpoints over the live set to find the ones it owns.
type Worker struct {
	ent.Schema
}

func (Worker) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("hostname"),
		field.Time("started_at"),
		field.Time("last_heartbeat"),
	}
}

func (Worker) Indexes() []ent.Index {
	return []ent.Index{index.Fields("last_heartbeat")}
}
