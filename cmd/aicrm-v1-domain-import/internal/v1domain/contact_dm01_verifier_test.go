package v1domain

import (
	"bytes"
	"context"
	"errors"
	"testing"

	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestDM01CustomerTagVerifierResolveAndVerifySameTransaction(t *testing.T) {
	verifier, target, uow, key := newDM01CustomerTagVerifierFixture(t)
	ctx := context.Background()
	customerID, err := verifier.ResolveVerifiedDM01Customer(ctx, "union-1")
	if err != nil || customerID != 19 || uow.calls != 1 {
		t.Fatalf("resolve customer=%d err=%v uow=%d", customerID, err, uow.calls)
	}
	if err = verifier.VerifyHistoricalTagCustomer(withDM01VerifierTransaction(ctx), "union-1", customerID); err != nil {
		t.Fatal(err)
	}
	if uow.calls != 1 || target.sourceLocks != 2 || target.lineageLocks != 2 || target.receiptReads != 2 || target.customerLocks != 2 || target.rootValidations != 2 {
		t.Fatalf("verify did not revalidate in the caller transaction: uow=%d target=%+v", uow.calls, target)
	}
	otherKey := bytes.Repeat([]byte{9}, len(key))
	wrongKeyVerifier, err := NewDM01CustomerTagVerifier(uow, target, otherKey, 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wrongKeyVerifier.ResolveVerifiedDM01Customer(ctx, "union-1"); !errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		t.Fatalf("wrong HMAC key err=%v, want blocked", err)
	}
}

func TestDM01CustomerTagVerifierBlocksWrongRunAndUnimportedReceipt(t *testing.T) {
	verifier, target, uow, key := newDM01CustomerTagVerifierFixture(t)
	wrongRun, err := NewDM01CustomerTagVerifier(uow, target, key, 8)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wrongRun.ResolveVerifiedDM01Customer(context.Background(), "union-1"); !errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		t.Fatalf("wrong run err=%v, want blocked", err)
	}
	target.receipt.Disposition = contactport.HistoricalImportQuarantined
	if _, err = verifier.ResolveVerifiedDM01Customer(context.Background(), "union-1"); !errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		t.Fatalf("unimported receipt err=%v, want blocked", err)
	}
}

func TestDM01CustomerTagVerifierBlocksLineageDriftAndMergedRoot(t *testing.T) {
	verifier, target, _, _ := newDM01CustomerTagVerifierFixture(t)
	target.lineage.PayloadHMAC = dm01VerifierDigest(9)
	if _, err := verifier.ResolveVerifiedDM01Customer(context.Background(), "union-1"); !errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		t.Fatalf("lineage drift err=%v, want blocked", err)
	}
	verifier, target, _, _ = newDM01CustomerTagVerifierFixture(t)
	target.rootErr = contactstore.ErrHistoricalImportTargetDrift
	if _, err := verifier.ResolveVerifiedDM01Customer(context.Background(), "union-1"); !errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		t.Fatalf("merged root err=%v, want blocked", err)
	}
}

func TestDM01CustomerTagVerifierPropagatesUnexpectedTargetError(t *testing.T) {
	verifier, target, _, _ := newDM01CustomerTagVerifierFixture(t)
	errDatabase := errors.New("temporary database failure")
	target.lineageErr = errDatabase
	if _, err := verifier.ResolveVerifiedDM01Customer(context.Background(), "union-1"); !errors.Is(err, errDatabase) || errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		t.Fatalf("unexpected target error=%v", err)
	}
}

func TestDM01CustomerTagVerifierPreflightRequiresImportedFullRunAndWitness(t *testing.T) {
	verifier, target, uow, key := newDM01CustomerTagVerifierFixture(t)
	archive := contactTagBindingArchive("union-1", "union-missing")
	if err := verifier.PreflightContactTagBindings(context.Background(), archive, "archive-run"); err != nil {
		t.Fatal(err)
	}
	if uow.calls != 1 || target.runReads != 1 || target.sourceLocks != 1 || target.lineageLocks != 1 || target.receiptReads != 1 || target.customerLocks != 1 || target.rootValidations != 1 {
		t.Fatalf("preflight calls uow=%d target=%+v", uow.calls, target)
	}
	target.mode = "incremental"
	if err := verifier.PreflightContactTagBindings(context.Background(), archive, "archive-run"); !errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		t.Fatalf("non-full run err=%v, want blocked", err)
	}
	target.mode = "full"
	target.state = "running"
	if err := verifier.PreflightContactTagBindings(context.Background(), archive, "archive-run"); !errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		t.Fatalf("non-imported run err=%v, want blocked", err)
	}
	target.state = "imported"
	wrongKeyVerifier, err := NewDM01CustomerTagVerifier(uow, target, bytes.Repeat([]byte{9}, len(key)), 7)
	if err != nil {
		t.Fatal(err)
	}
	if err = wrongKeyVerifier.PreflightContactTagBindings(context.Background(), archive, "archive-run"); !errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		t.Fatalf("wrong key preflight err=%v, want blocked", err)
	}
	if err = verifier.PreflightContactTagBindings(context.Background(), contactTagBindingArchive("union-missing"), "archive-run"); !errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		t.Fatalf("missing witness err=%v, want blocked", err)
	}
}

func TestDM01CustomerTagVerifierPreflightPreservesRunReadError(t *testing.T) {
	verifier, target, _, _ := newDM01CustomerTagVerifierFixture(t)
	errDatabase := errors.New("temporary run read failure")
	target.runErr = errDatabase
	if err := verifier.PreflightContactTagBindings(context.Background(), contactTagBindingArchive("union-1"), "archive-run"); !errors.Is(err, errDatabase) || errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		t.Fatalf("run read error=%v", err)
	}
}

type dm01VerifierTransactionKey struct{}

func withDM01VerifierTransaction(ctx context.Context) context.Context {
	return context.WithValue(ctx, dm01VerifierTransactionKey{}, true)
}

type dm01VerifierUOW struct{ calls int }

func (uow *dm01VerifierUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	return callback(withDM01VerifierTransaction(ctx))
}

type dm01VerifierTarget struct {
	contactport.HistoricalImportTarget
	expectedKey [32]byte
	lineage     contactport.HistoricalImportLineage
	receipt     contactport.HistoricalImportRowReceipt

	lineageFound, receiptFound                                      bool
	mode, state                                                     string
	sourceErr, lineageErr, receiptErr, customerErr, rootErr, runErr error
	sourceLocks, lineageLocks, receiptReads, runReads               int
	customerLocks, rootValidations                                  int
}

func (target *dm01VerifierTarget) LockHistoricalImportSource(ctx context.Context, _ contactport.HistoricalImportSource, _ []byte) error {
	if err := dm01VerifierTransaction(ctx); err != nil {
		return err
	}
	target.sourceLocks++
	return target.sourceErr
}

func (target *dm01VerifierTarget) LockHistoricalImportLineage(ctx context.Context, _ contactport.HistoricalImportSource, key []byte) (contactport.HistoricalImportLineage, bool, error) {
	if err := dm01VerifierTransaction(ctx); err != nil {
		return contactport.HistoricalImportLineage{}, false, err
	}
	target.lineageLocks++
	if target.lineageErr != nil {
		return contactport.HistoricalImportLineage{}, false, target.lineageErr
	}
	if !bytes.Equal(key, target.expectedKey[:]) {
		return contactport.HistoricalImportLineage{}, false, nil
	}
	return target.lineage, target.lineageFound, nil
}

func (target *dm01VerifierTarget) FindHistoricalImportRowReceipt(ctx context.Context, runID int64, _ contactport.HistoricalImportSource, key []byte) (contactport.HistoricalImportRowReceipt, bool, error) {
	if err := dm01VerifierTransaction(ctx); err != nil {
		return contactport.HistoricalImportRowReceipt{}, false, err
	}
	target.receiptReads++
	if target.receiptErr != nil {
		return contactport.HistoricalImportRowReceipt{}, false, target.receiptErr
	}
	if runID != target.lineage.LastRunID || !bytes.Equal(key, target.expectedKey[:]) {
		return contactport.HistoricalImportRowReceipt{}, false, nil
	}
	return target.receipt, target.receiptFound, nil
}

func (target *dm01VerifierTarget) LockHistoricalImportCustomerTarget(ctx context.Context, customerID int64) (contactport.HistoricalImportCustomerFact, error) {
	if err := dm01VerifierTransaction(ctx); err != nil {
		return contactport.HistoricalImportCustomerFact{}, err
	}
	target.customerLocks++
	if target.customerErr != nil {
		return contactport.HistoricalImportCustomerFact{}, target.customerErr
	}
	if customerID != target.lineage.TargetID {
		return contactport.HistoricalImportCustomerFact{}, contactstore.ErrHistoricalImportTargetDrift
	}
	return contactport.HistoricalImportCustomerFact{}, nil
}

func (target *dm01VerifierTarget) ValidateHistoricalImportCustomerRoot(ctx context.Context, customerID int64) error {
	if err := dm01VerifierTransaction(ctx); err != nil {
		return err
	}
	target.rootValidations++
	if target.rootErr != nil {
		return target.rootErr
	}
	if customerID != target.lineage.TargetID {
		return contactstore.ErrHistoricalImportTargetDrift
	}
	return nil
}

func (target *dm01VerifierTarget) ReadHistoricalImportRun(ctx context.Context, runID int64) (string, string, error) {
	if err := dm01VerifierTransaction(ctx); err != nil {
		return "", "", err
	}
	target.runReads++
	if target.runErr != nil {
		return "", "", target.runErr
	}
	if runID != target.lineage.LastRunID {
		return "", "", contactstore.ErrHistoricalImportTargetDrift
	}
	return target.mode, target.state, nil
}

func dm01VerifierTransaction(ctx context.Context) error {
	if active, _ := ctx.Value(dm01VerifierTransactionKey{}).(bool); !active {
		return errors.New("DM01 verifier target requires caller transaction")
	}
	return nil
}

func newDM01CustomerTagVerifierFixture(t *testing.T) (*DM01CustomerTagVerifier, *dm01VerifierTarget, *dm01VerifierUOW, []byte) {
	t.Helper()
	key := bytes.Repeat([]byte{4}, 32)
	sourceKey, err := contactmigration.SourceKeyHMAC(key, dm01CustomerIdentitySourceTable, "union-1")
	if err != nil {
		t.Fatal(err)
	}
	var source [32]byte
	copy(source[:], sourceKey)
	payload, field := dm01VerifierDigest(1), dm01VerifierDigest(2)
	target := &dm01VerifierTarget{expectedKey: source, lineageFound: true, receiptFound: true, mode: "full", state: "imported",
		lineage: contactport.HistoricalImportLineage{TargetID: 19, PayloadHMAC: payload, FieldDigest: field, LastRunID: 7},
		receipt: contactport.HistoricalImportRowReceipt{PayloadHMAC: payload, FieldDigest: field, Disposition: contactport.HistoricalImportImported}}
	uow := &dm01VerifierUOW{}
	verifier, err := NewDM01CustomerTagVerifier(uow, target, key, 7)
	if err != nil {
		t.Fatal(err)
	}
	return verifier, target, uow, key
}

func dm01VerifierDigest(seed byte) []byte { return bytes.Repeat([]byte{seed}, 32) }

func contactTagBindingArchive(unionIDs ...string) *contactArchive {
	rows := make([]v1archive.ArchivedRow, 0, len(unionIDs))
	for index, unionID := range unionIDs {
		rows = append(rows, contactArchivedRow(contactBindingsTable, byte(index+1), map[string]any{"unionid": unionID}))
	}
	return &contactArchive{rows: map[string][]v1archive.ArchivedRow{contactBindingsTable: rows}}
}
