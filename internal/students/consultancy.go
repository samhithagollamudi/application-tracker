package students

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/ChandraShekar2001/application-tracker/internal/catalog"
)

// ConsultancyChoice is what the agent network picked in the "Referred by"
// dropdown: an existing consultancy, or "Other" with a typed name (which is
// added to the admin list). Both zero means none.
type ConsultancyChoice struct {
	ID        int64
	OtherName string
}

func (c ConsultancyChoice) resolve(ctx context.Context, tx pgx.Tx, actorID int64) (*int64, error) {
	if name := strings.Join(strings.Fields(c.OtherName), " "); name != "" {
		if utf8.RuneCountInString(name) > 200 {
			return nil, FieldErrors{"consultancy": "Keep the consultancy name under 200 characters."}
		}
		id, err := catalog.EnsureConsultancy(ctx, tx, name, actorID)
		return &id, err
	}
	if c.ID == 0 {
		return nil, nil
	}
	ok, err := catalog.ConsultancyExists(ctx, tx, c.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, FieldErrors{"consultancy": "Pick a consultancy from the list, or choose Other."}
	}
	id := c.ID
	return &id, nil
}
