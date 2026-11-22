// Package activity writes and reads the per-student activity log.
package activity

import (
	"context"
	"encoding/json"

	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
)

// Action codes stored in activity_log.action.
const (
	StudentCreated     = "student.created"
	StudentUpdated     = "student.updated"
	StudentMoved       = "student.moved"
	ApplicationAdded   = "application.added"
	ApplicationEdited  = "application.edited"
	ApplicationStatus  = "application.status"
	ApplicationDeleted = "application.deleted"
)

type detail struct {
	Text string `json:"text"`
}

// Log records one entry. Pass a transaction-bound DBTX so the entry commits
// (or rolls back) together with the change it describes.
func Log(ctx context.Context, db gen.DBTX, studentID, actorID int64, action, text string) error {
	d, _ := json.Marshal(detail{Text: text})
	return gen.New(db).InsertActivity(ctx, gen.InsertActivityParams{
		StudentID: &studentID, ActorID: actorID, Action: action, Detail: d,
	})
}

// List returns the latest entries for a student. The caller must already
// have checked the student is in scope.
func List(ctx context.Context, db gen.DBTX, studentID int64) ([]model.Activity, error) {
	rows, err := gen.New(db).ListActivity(ctx, &studentID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Activity, 0, len(rows))
	for _, r := range rows {
		var d detail
		_ = json.Unmarshal(r.Detail, &d)
		if d.Text == "" {
			d.Text = r.Action
		}
		out = append(out, model.Activity{Text: d.Text, ActorName: r.ActorName, At: r.CreatedAt})
	}
	return out, nil
}
