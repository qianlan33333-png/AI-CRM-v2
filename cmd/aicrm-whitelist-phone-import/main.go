package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	usage       = "Usage: aicrm-whitelist-phone-import --mode=<plan|apply> [--run-id=<id>] [--source-digest=<sha256>]"
	phoneSource = "v1.crm_user_identity.current_phone"
)

var (
	chinaMobile = regexp.MustCompile(`^1[3-9][0-9]{9}$`)
	e164Phone   = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
)

type sourceRow struct {
	unionID, externalUserID, mobile, source, updatedAt string
	verified                                           bool
}

type disposition string

const (
	migrate          disposition = "MIGRATE"
	rejectInvalid    disposition = "REJECT_INVALID"
	rejectUnverified disposition = "REJECT_UNVERIFIED"
	rejectUnmatched  disposition = "REJECT_UNMATCHED"
	rejectConflict   disposition = "REJECT_CONFLICT"
)

type classifiedRow struct {
	sourceRow
	phone       string
	customerID  int64
	disposition disposition
	reason      string
}

type report struct {
	Mode               string `json:"mode"`
	SourceDigest       string `json:"source_digest"`
	SourceRows         int    `json:"source_rows"`
	MigratedRows       int    `json:"migrated_rows"`
	InsertedIdentities int    `json:"inserted_identities"`
	ExistingIdentities int    `json:"existing_identities"`
	RejectedInvalid    int    `json:"rejected_invalid"`
	RejectedUnverified int    `json:"rejected_unverified"`
	RejectedUnmatched  int    `json:"rejected_unmatched"`
	RejectedConflict   int    `json:"rejected_conflict"`
	IDPhoneCustomers   int    `json:"id_phone_customers"`
	CreatedCustomers   int    `json:"created_customers"`
	ReusedCustomers    int    `json:"reused_customers"`
	CreatedReferences  int    `json:"created_references"`
	ExistingReferences int    `json:"existing_references"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.LookupEnv); err != nil {
		fmt.Fprintln(os.Stderr, "aicrm-whitelist-phone-import:", err)
		os.Exit(2)
	}
}

func run(ctx context.Context, args []string, input io.Reader, output io.Writer, lookup func(string) (string, bool)) error {
	flags := flag.NewFlagSet("aicrm-whitelist-phone-import", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	mode := flags.String("mode", "", "plan|apply")
	runID := flags.String("run-id", "", "whitelist phone import run id")
	expectedDigest := flags.String("source-digest", "", "expected source SHA-256 for apply")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*mode != "plan" && *mode != "apply") {
		return errors.New(usage)
	}
	if *mode == "apply" && (!validRunID(*runID) || !isSHA256(*expectedDigest)) {
		return errors.New("apply requires --run-id=wli_phone_<id> and a lowercase SHA-256 --source-digest")
	}
	databaseURL, ok := lookup("AICRM_DATABASE_URL")
	if !ok || strings.TrimSpace(databaseURL) == "" {
		return errors.New("AICRM_DATABASE_URL is required")
	}
	keyValue, ok := lookup("AICRM_IDENTITY_HMAC_KEY")
	key, err := decodeHMACKey(keyValue, ok)
	if err != nil {
		return err
	}
	rows, digest, err := readSource(input)
	if err != nil {
		return err
	}
	if *mode == "apply" && digest != *expectedDigest {
		return errors.New("source digest changed after plan")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return errors.New("target database unavailable")
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	access := pgx.ReadOnly
	if *mode == "apply" {
		access = pgx.ReadWrite
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable, AccessMode: access})
	if err != nil {
		return errors.New("target database unavailable")
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var databaseName string
	if err = tx.QueryRow(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil || databaseName != "aicrm_v2_core" {
		return errors.New("target must be aicrm_v2_core")
	}
	classified, result, err := classify(ctx, tx, rows, digest)
	if err != nil {
		return err
	}
	result.Mode = *mode
	if *mode == "apply" {
		if err = apply(ctx, tx, *runID, classified, &result, key); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return json.NewEncoder(output).Encode(result)
}

func readSource(input io.Reader) ([]sourceRow, string, error) {
	reader := csv.NewReader(input)
	reader.FieldsPerRecord = 6
	records, err := reader.ReadAll()
	if err != nil || len(records) < 1 {
		return nil, "", errors.New("invalid source CSV")
	}
	wantHeader := []string{"unionid", "primary_external_userid", "mobile_normalized", "mobile_verified", "mobile_source", "updated_at"}
	if strings.Join(records[0], "\x00") != strings.Join(wantHeader, "\x00") {
		return nil, "", errors.New("unexpected source CSV header")
	}
	rows := make([]sourceRow, 0, len(records)-1)
	seenUnionIDs := make(map[string]struct{}, len(records)-1)
	seenExternalIDs := make(map[string]struct{}, len(records)-1)
	for _, record := range records[1:] {
		unionID := strings.TrimSpace(record[0])
		if unionID == "" || len(unionID) > 512 {
			return nil, "", errors.New("source row has invalid unionid")
		}
		if _, duplicate := seenUnionIDs[unionID]; duplicate {
			return nil, "", errors.New("source CSV contains duplicate unionid")
		}
		seenUnionIDs[unionID] = struct{}{}
		externalUserID := strings.TrimSpace(record[1])
		if len(externalUserID) > 512 {
			return nil, "", errors.New("source row has invalid external_userid")
		}
		if externalUserID != "" {
			if _, duplicate := seenExternalIDs[externalUserID]; duplicate {
				return nil, "", errors.New("source CSV contains duplicate external_userid")
			}
			seenExternalIDs[externalUserID] = struct{}{}
		}
		verified := record[3] == "t" || record[3] == "true"
		if !verified && record[3] != "f" && record[3] != "false" {
			return nil, "", errors.New("source row has invalid verification state")
		}
		rows = append(rows, sourceRow{unionID: unionID, externalUserID: externalUserID, mobile: strings.TrimSpace(record[2]), verified: verified, source: strings.TrimSpace(record[4]), updatedAt: strings.TrimSpace(record[5])})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].unionID < rows[j].unionID })
	hash := sha256.New()
	for _, row := range rows {
		for _, value := range []string{row.unionID, row.externalUserID, row.mobile, fmt.Sprint(row.verified), row.source, row.updatedAt} {
			_, _ = fmt.Fprintf(hash, "%d:%s", len(value), value)
		}
	}
	return rows, hex.EncodeToString(hash.Sum(nil)), nil
}

func classify(ctx context.Context, tx pgx.Tx, rows []sourceRow, digest string) ([]classifiedRow, report, error) {
	result := report{SourceDigest: digest, SourceRows: len(rows)}
	classified := make([]classifiedRow, 0, len(rows))
	groups := map[string][]int{}
	for _, row := range rows {
		item := classifiedRow{sourceRow: row}
		switch {
		case !row.verified:
			item.disposition, item.reason = rejectUnverified, "source phone is not verified"
		case normalizePhone(row.mobile) == "":
			item.disposition, item.reason = rejectInvalid, "source phone is not a supported E.164 mobile"
		default:
			item.phone = normalizePhone(row.mobile)
			item.disposition = migrate
			groups[item.phone] = append(groups[item.phone], len(classified))
		}
		classified = append(classified, item)
	}
	phones := make([]string, 0, len(groups))
	for phone := range groups {
		phones = append(phones, phone)
	}
	sort.Strings(phones)
	nextPlannedCustomerID := int64(-1)
	for _, phone := range phones {
		candidates := map[int64]struct{}{}
		for _, index := range groups[phone] {
			item := &classified[index]
			for _, reference := range []struct{ entity, value string }{{"unionid", item.unionID}, {"wecom_external_userid", item.externalUserID}} {
				if reference.value == "" {
					continue
				}
				customerID, found, lookupErr := lookupReferenceCustomer(ctx, tx, reference.entity, reference.value)
				if lookupErr != nil {
					return nil, report{}, lookupErr
				}
				if found {
					candidates[customerID] = struct{}{}
				}
			}
		}
		customerID, found, lookupErr := lookupPhoneCustomer(ctx, tx, phone)
		if lookupErr != nil {
			return nil, report{}, lookupErr
		}
		if found {
			candidates[customerID] = struct{}{}
		}
		switch len(candidates) {
		case 0:
			customerID = nextPlannedCustomerID
			nextPlannedCustomerID--
		case 1:
			for customerID = range candidates {
			}
		default:
			for _, index := range groups[phone] {
				classified[index].disposition = rejectConflict
				classified[index].reason = "stable IDs and verified phone resolve to different V2 customers"
			}
			continue
		}
		for _, index := range groups[phone] {
			classified[index].customerID = customerID
		}
	}
	customerPhones := map[int64]map[string]struct{}{}
	for index := range classified {
		item := &classified[index]
		if item.disposition != migrate || item.customerID <= 0 {
			continue
		}
		if customerPhones[item.customerID] == nil {
			customerPhones[item.customerID] = map[string]struct{}{}
		}
		customerPhones[item.customerID][item.phone] = struct{}{}
	}
	for index := range classified {
		item := &classified[index]
		if item.disposition == migrate && item.customerID > 0 && len(customerPhones[item.customerID]) != 1 {
			item.disposition, item.reason = rejectConflict, "stable V2 customer resolves to multiple verified phones"
		}
		switch item.disposition {
		case migrate:
			result.MigratedRows++
		case rejectInvalid:
			result.RejectedInvalid++
		case rejectUnverified:
			result.RejectedUnverified++
		case rejectUnmatched:
			result.RejectedUnmatched++
		case rejectConflict:
			result.RejectedConflict++
		}
	}
	for _, phone := range phones {
		if classified[groups[phone][0]].disposition == migrate {
			result.IDPhoneCustomers++
			if classified[groups[phone][0]].customerID < 0 {
				result.CreatedCustomers++
			} else {
				result.ReusedCustomers++
			}
		}
	}
	return classified, result, nil
}

func lookupReferenceCustomer(ctx context.Context, tx pgx.Tx, entity, value string) (int64, bool, error) {
	digest := sha256.Sum256([]byte("aicrm_v2_frozen\x00" + entity + "\x00" + value))
	var customerID int64
	err := tx.QueryRow(ctx, `SELECT customer_id FROM public.source_subject_refs WHERE source_system='aicrm_v2_frozen' AND source_entity=$1 AND reference_digest=$2`, entity, digest[:]).Scan(&customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return customerID, err == nil, err
}

func lookupPhoneCustomer(ctx context.Context, tx pgx.Tx, phone string) (int64, bool, error) {
	var customerID int64
	err := tx.QueryRow(ctx, `SELECT customer_id FROM public.identities WHERE kind='phone' AND scope='phone:e164' AND normalized_value=$1`, phone).Scan(&customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return customerID, err == nil, err
}

func apply(ctx context.Context, tx pgx.Tx, runID string, rows []classifiedRow, result *report, key []byte) error {
	digest, _ := hex.DecodeString(result.SourceDigest)
	if _, err := tx.Exec(ctx, `INSERT INTO public.whitelist_import_runs(id,source_digest,state,started_at) VALUES($1,$2,'running',now())`, runID, digest); err != nil {
		return errors.New("phone import run already exists")
	}
	createdCustomerIDs := map[int64]int64{}
	for _, row := range rows {
		if row.disposition != migrate || row.customerID >= 0 {
			continue
		}
		if _, found := createdCustomerIDs[row.customerID]; found {
			continue
		}
		var customerID int64
		if err := tx.QueryRow(ctx, `INSERT INTO public.customers(state,created_at,updated_at) VALUES('active',now(),now()) RETURNING id`).Scan(&customerID); err != nil {
			return err
		}
		createdCustomerIDs[row.customerID] = customerID
	}
	identityIDs := map[string]int64{}
	for _, row := range rows {
		var targetID *string
		dispositionValue := "REJECT"
		reason := row.reason
		if row.disposition == migrate {
			customerID := row.customerID
			if customerID < 0 {
				customerID = createdCustomerIDs[customerID]
			}
			for _, reference := range []struct{ entity, value string }{{"unionid", row.unionID}, {"wecom_external_userid", row.externalUserID}} {
				if reference.value == "" {
					continue
				}
				referenceDigest := sha256.Sum256([]byte("aicrm_v2_frozen\x00" + reference.entity + "\x00" + reference.value))
				var inserted bool
				err := tx.QueryRow(ctx, `
INSERT INTO public.source_subject_refs AS existing(customer_id,source_system,source_entity,reference_digest,assurance,created_at)
VALUES($1,'aicrm_v2_frozen',$2,$3,'verified',now())
ON CONFLICT(source_system,source_entity,reference_digest) DO UPDATE SET customer_id=EXCLUDED.customer_id
WHERE existing.customer_id=EXCLUDED.customer_id
RETURNING (xmax=0)`, customerID, reference.entity, referenceDigest[:]).Scan(&inserted)
				if errors.Is(err, pgx.ErrNoRows) {
					return errors.New("target stable ID changed after plan")
				}
				if err != nil {
					return err
				}
				if inserted {
					result.CreatedReferences++
				} else {
					result.ExistingReferences++
				}
			}
			identityID, found := identityIDs[row.phone]
			if !found {
				fingerprint := hmacDigest(key, "phone:e164\x00"+row.phone)[:16]
				err := tx.QueryRow(ctx, `
INSERT INTO public.identities AS existing(customer_id,kind,scope,normalized_value,normalizer_version,assurance,source,review_fingerprint,fingerprint_key_version,bound_at)
VALUES($1,'phone','phone:e164',$2,1,'verified',$3,$4,1,now())
ON CONFLICT(kind,scope,normalized_value) DO UPDATE SET
  customer_id=COALESCE(existing.customer_id,EXCLUDED.customer_id),
  assurance='verified',source=EXCLUDED.source,review_fingerprint=EXCLUDED.review_fingerprint,
  fingerprint_key_version=1,bound_at=COALESCE(existing.bound_at,EXCLUDED.bound_at)
WHERE existing.customer_id IS NULL OR existing.customer_id=EXCLUDED.customer_id
RETURNING id,(xmax=0)`, customerID, row.phone, phoneSource, fingerprint).Scan(&identityID, &found)
				if errors.Is(err, pgx.ErrNoRows) {
					return errors.New("target phone identity changed after plan")
				}
				if err != nil {
					return err
				}
				identityIDs[row.phone] = identityID
				if found {
					result.InsertedIdentities++
				} else {
					result.ExistingIdentities++
				}
			}
			value := fmt.Sprint(identityID)
			targetID, dispositionValue, reason = &value, "MIGRATE", ""
		}
		sourceKey := hmacDigest(key, "crm_user_identity\x00"+row.unionID)
		payload := hmacDigest(key, strings.Join([]string{row.unionID, row.externalUserID, row.mobile, fmt.Sprint(row.verified), row.source, row.updatedAt}, "\x00"))
		if _, err := tx.Exec(ctx, `INSERT INTO public.whitelist_import_domain_receipts(run_id,domain,source_entity,source_key_digest,source_payload_digest,target_entity,target_id,disposition,reason,created_at) VALUES($1,'identity',$2,$3,$4,'identities',$5,$6,$7,now())`, runID, phoneSource, sourceKey, payload, targetID, dispositionValue, reason); err != nil {
			return err
		}
	}
	if result.SourceRows != result.MigratedRows+result.RejectedInvalid+result.RejectedUnverified+result.RejectedUnmatched+result.RejectedConflict {
		return errors.New("phone reconciliation silently dropped rows")
	}
	reportJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE public.whitelist_import_runs SET state='completed',completed_at=now(),report=$2 WHERE id=$1 AND state='running'`, runID, reportJSON)
	return err
}

func normalizePhone(value string) string {
	value = strings.TrimSpace(value)
	if chinaMobile.MatchString(value) {
		return "+86" + value
	}
	if e164Phone.MatchString(value) {
		return value
	}
	return ""
}

func hmacDigest(key []byte, value string) []byte {
	hash := hmac.New(sha256.New, key)
	_, _ = hash.Write([]byte(value))
	return hash.Sum(nil)
}

func decodeHMACKey(value string, present bool) ([]byte, error) {
	if !present || value == "" {
		return nil, errors.New("AICRM_IDENTITY_HMAC_KEY is required")
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, errors.New("AICRM_IDENTITY_HMAC_KEY must be canonical 32-byte base64url")
	}
	return decoded, nil
}

func validRunID(value string) bool {
	if !strings.HasPrefix(value, "wli_phone_") || len(value) > 84 || len(value) < 18 {
		return false
	}
	for _, char := range value[len("wli_phone_"):] {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func isSHA256(value string) bool {
	_, err := hex.DecodeString(value)
	return len(value) == sha256.Size*2 && err == nil && value == strings.ToLower(value)
}
