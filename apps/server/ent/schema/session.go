package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Session is a signed-in viewer. The web app's cookie holds a random ID and
// this row is keyed by its SHA-256, so the table can't be used to sign in.
type Session struct {
	ent.Schema
}

func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().NotEmpty(),
		field.String("subject").Immutable(),
		field.String("name").Immutable(),
		// So an admin can tell who a session belongs to: OIDC subjects are
		// opaque IDs and names aren't unique. Empty when the provider didn't
		// send them (and email only when verified).
		field.String("email").Default("").Immutable(),
		field.String("username").Default("").Immutable(),
		field.Strings("groups").Optional().Immutable(),
		field.String("user_agent").Default("").Immutable(),
		field.String("ip").Default("").Immutable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("last_seen_at").Default(time.Now),
		field.Time("expires_at").Immutable(),
		field.Time("revoked_at").Optional().Nillable(),
	}
}

func (Session) Indexes() []ent.Index {
	return []ent.Index{index.Fields("subject"), index.Fields("expires_at")}
}
