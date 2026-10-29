package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// ConfigVersion is a single row recording the config currently synced to the
// DB. A replica applies its config when its generation is higher, or equal
// with a different hash (so the most recently started replica wins).
type ConfigVersion struct {
	ent.Schema
}

func (ConfigVersion) Fields() []ent.Field {
	return []ent.Field{
		field.String("hash"),
		field.Int64("generation"),
		field.String("applied_by"),
		field.Time("applied_at").Default(time.Now),
	}
}
