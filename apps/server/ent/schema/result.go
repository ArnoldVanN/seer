package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Result is one raw check. (endpoint_id, slot) is unique: a slot is one
// interval-sized window, so two workers that both think they own an endpoint
// during a handoff still store only one result for it.
type Result struct {
	ent.Schema
}

func (Result) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int("endpoint_id"),
		field.Int64("slot"),
		field.Time("checked_at"),
		field.Bool("success"),
		field.Int64("response_time_ms"),
		field.String("error").Default(""),
		field.JSON("conditions", []ConditionResult{}).Optional(),
	}
}

// ConditionResult records how one configured condition evaluated.
type ConditionResult struct {
	Condition string `json:"condition"`
	Success   bool   `json:"success"`
}

func (Result) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("endpoint", Endpoint.Type).Ref("results").Field("endpoint_id").Unique().Required(),
	}
}

func (Result) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("endpoint_id", "slot").Unique(),
		index.Fields("endpoint_id", "checked_at"),
		index.Fields("checked_at"),
	}
}
