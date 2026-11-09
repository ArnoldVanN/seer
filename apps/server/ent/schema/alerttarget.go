package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// AlertTarget is a named place to send alerts, created through the admin
// API. Endpoint alerts name it as their target.
type AlertTarget struct {
	ent.Schema
}

func (AlertTarget) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").Unique().NotEmpty(),
		field.Bytes("spec").Comment("JSON-encoded config.AlertTarget"),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
