package engine

import (
	"errors"
	"testing"
)

// fakeBackupStepper lets TestRunBackupAlwaysCallsFinish drive runBackup
// without a real SQLite connection, so the one thing runBackup exists to
// guarantee -- Finish always runs, even when Step fails -- can be
// verified directly instead of through hard-to-reproduce SQLite failure
// conditions (backup.go's runBackup doc comment explains why this matters:
// Finish is the only call that releases the source connection NewRestore
// opens internally).
type fakeBackupStepper struct {
	stepMore  bool
	stepErr   error
	finishErr error

	finishCalled bool
}

func (f *fakeBackupStepper) Step(n int32) (bool, error) {
	return f.stepMore, f.stepErr
}

func (f *fakeBackupStepper) Finish() error {
	f.finishCalled = true
	return f.finishErr
}

func TestRunBackupAlwaysCallsFinish(t *testing.T) {
	cases := []struct {
		name      string
		stepMore  bool
		stepErr   error
		finishErr error
		wantErr   bool
	}{
		{name: "step succeeds and completes", stepMore: false, stepErr: nil, wantErr: false},
		{name: "step fails", stepErr: errors.New("boom"), wantErr: true},
		{name: "step reports more pages remaining", stepMore: true, wantErr: true},
		{name: "step succeeds but finish fails", finishErr: errors.New("finish boom"), wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeBackupStepper{stepMore: c.stepMore, stepErr: c.stepErr, finishErr: c.finishErr}
			err := runBackup(fake)

			if !fake.finishCalled {
				t.Fatalf("runBackup did not call Finish (stepErr=%v stepMore=%v)", c.stepErr, c.stepMore)
			}
			if (err != nil) != c.wantErr {
				t.Fatalf("runBackup() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

// TestMapBusyWrapsErrBusyOnSQLiteBusyCode forces a real SQLITE_BUSY from
// modernc.org/sqlite (rather than a stub: *sqlite.Error has unexported
// fields, so nothing outside the driver package can construct one) by
// holding a write transaction open on one connection and giving a second
// connection to the same live database a busy_timeout of 0, so its
// conflicting write fails immediately instead of waiting.
func TestMapBusyWrapsErrBusyOnSQLiteBusyCode(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()

	blocker, err := db.sdb.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	if _, err := blocker.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer blocker.ExecContext(ctx, "ROLLBACK")

	conn, err := db.sdb.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout=0"); err != nil {
		t.Fatal(err)
	}

	_, rawErr := conn.ExecContext(ctx, "BEGIN IMMEDIATE")
	if rawErr == nil {
		t.Fatal("expected the conflicting BEGIN IMMEDIATE to fail with SQLITE_BUSY")
	}

	got := mapBusy(rawErr)
	if !errors.Is(got, ErrBusy) {
		t.Fatalf("mapBusy(%v) = %v, want an error wrapping ErrBusy", rawErr, got)
	}
}

func TestMapBusyPassesThroughOtherErrors(t *testing.T) {
	other := errors.New("some other failure")
	got := mapBusy(other)
	if got != other {
		t.Fatalf("mapBusy(other) = %v, want the original error unchanged", got)
	}
	if mapBusy(nil) != nil {
		t.Fatalf("mapBusy(nil) should stay nil")
	}
}

// TestBackupFinishesEvenOnStepFailure is an integration-level regression
// test for the leak runBackup exists to prevent: NewRestore's Backup
// closes the source connection it opened internally only from Finish
// (backup.go). This forces a real Step failure (an invalid, nonexistent
// source path -- NewRestore itself succeeds since it only opens the
// connection lazily, but Step fails once it tries to actually read the
// source) and then confirms the call returns an error rather than
// hanging or leaving the live database unusable, which is what would
// happen if Finish's rollback of Step's write transaction were skipped.
func TestBackupFinishesEvenOnStepFailure(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	conn, err := db.sdb.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// A source path with a directory component that does not exist:
	// NewRestore succeeds (it just records the URI), but the underlying
	// sqlite3_backup_step fails once it tries to open/read the source.
	if err := restoreFrom(conn, "/nonexistent-dir-for-spike/does-not-exist.sqlite"); err == nil {
		t.Fatal("restoreFrom with a nonexistent source: expected an error, got nil")
	}

	// If Finish still ran (rolling back whatever write transaction Step
	// may have opened on the destination and releasing the source
	// connection), the live database is left usable.
	if _, err := db.Exec("CREATE TABLE ok(x)"); err != nil {
		t.Fatalf("write after failed restoreFrom: %v (Backup.Finish likely was not called on Step failure)", err)
	}
}
