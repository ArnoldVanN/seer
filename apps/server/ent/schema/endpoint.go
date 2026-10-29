package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Endpoint is a check synced from the config file. The full definition lives
// in Spec so new check options don't need a migration.
type Endpoint struct {
	ent.Schema
}

func (Endpoint) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").Unique().NotEmpty(),
		field.String("name"),
		field.String("group_name").Default(""),
		field.String("type"),
		field.Int("interval_seconds").Positive(),
		field.Bool("enabled").Default(true),
		field.Bytes("spec").Comment("JSON-encoded config.Endpoint"),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Endpoint) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("state", EndpointState.Type).Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("results", Result.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("rollups", Rollup.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
