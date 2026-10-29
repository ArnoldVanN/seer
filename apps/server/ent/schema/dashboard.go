package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type Dashboard struct {
	ent.Schema
}

func (Dashboard) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").Unique().NotEmpty(),
		field.String("title"),
		field.Enum("visibility").Values("public", "private"),
		field.Int("position"),
		field.Strings("groups").Optional(),
		field.Strings("endpoint_keys").Optional(),
		field.Strings("allowed_groups").Optional().
			Comment("Private dashboards only: auth groups allowed to view it. Empty means any signed-in user."),
	}
}
