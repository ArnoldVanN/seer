package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Rollup aggregates results into fixed buckets so dashboards never scan raw
// results. Resolutions are 5m (from results), 1h (from 5m) and 1d (from 1h),
// each with its own retention.
type Rollup struct {
	ent.Schema
}

func (Rollup) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int("endpoint_id"),
		field.Enum("resolution").Values("5m", "1h", "1d"),
		field.Time("bucket"),
		field.Int64("checks"),
		field.Int64("successes"),
		// Response times of successful checks only; 0 without any.
		field.Int64("rt_sum_ms"),
		field.Int64("rt_min_ms"),
		field.Int64("rt_max_ms"),
	}
}

func (Rollup) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("endpoint", Endpoint.Type).Ref("rollups").Field("endpoint_id").Unique().Required(),
	}
}

func (Rollup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("endpoint_id", "resolution", "bucket").Unique(),
		index.Fields("resolution", "bucket"),
	}
}
