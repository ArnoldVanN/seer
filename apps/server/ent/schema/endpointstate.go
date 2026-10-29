package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// EndpointState is the current status of an endpoint. Workers update it with
// an optimistic version check, so a transition (and its notification) is
// applied by exactly one worker even if two briefly own the endpoint.
type EndpointState struct {
	ent.Schema
}

func (EndpointState) Fields() []ent.Field {
	return []ent.Field{
		field.Int("endpoint_id").Unique(),
		field.Enum("status").Values("unknown", "up", "down").Default("unknown"),
		field.Int64("version").Default(0),
		field.Int("consecutive_failures").Default(0),
		field.Int("consecutive_successes").Default(0),
		field.Time("last_checked_at").Optional().Nillable(),
		field.Int64("last_response_time_ms").Default(0),
		field.String("last_error").Default(""),
		field.Time("changed_at").Default(time.Now),
		field.JSON("triggered_alerts", []bool{}).Optional().
			Comment("Per alert index: whether it is currently triggered"),
	}
}

func (EndpointState) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("endpoint", Endpoint.Type).Ref("state").Field("endpoint_id").Unique().Required(),
	}
}
