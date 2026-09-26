package provisioning

import (
	"database/sql"
	"testing"
	"time"
)

func TestStepAttemptCompletionUsesLatestAttempt(t *testing.T) {
	old := time.Now()
	latest := old.Add(time.Minute)
	if stepAttemptCompleted(sql.NullTime{Time: latest, Valid: true}, sql.NullTime{Time: old, Valid: true}, sql.NullString{}, false) {
		t.Fatal("older finish cannot complete newer attempt")
	}
	if !stepAttemptCompleted(sql.NullTime{Time: old, Valid: true}, sql.NullTime{Time: latest, Valid: true}, sql.NullString{}, false) {
		t.Fatal("latest completed attempt not recognized")
	}
}
