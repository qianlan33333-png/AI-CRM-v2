package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

var p1CandidateOperations = map[string]bool{
	"listCustomers": true, "getCustomer": true, "updateCustomer": true,
	"listCustomerEvents": true, "resolveIdentity": true, "bindIdentity": true,
	"ingestIdentityEvent": true, "getAuthSession": true, "logoutAdmin": true,
	"getAdminConfigOverview": true,
}

var p2StageOperations = map[string]bool{
	"listStages": true, "createStage": true, "renameStage": true,
}

var p3ContactOperations = map[string]bool{
	"listTags": true, "setCustomerStage": true,
	"addCustomerTag": true, "removeCustomerTag": true,
}

var p3IdentityOperations = map[string]bool{
	"listIdentityMergeReviews": true, "approveIdentityMergeReview": true,
	"rejectIdentityMergeReview": true,
}

var p3SegmentOperations = map[string]bool{
	"listSegments": true, "createSegment": true, "getSegment": true,
	"updateSegment": true, "listSegmentMembers": true, "requestSegmentRefresh": true,
}

var p4AutomationOperations = map[string]bool{
	"listAutomationTriggerRuns": true,
}

var p4ProductOperations = map[string]bool{
	"listProducts": true, "createProduct": true, "getProduct": true,
}

var p4ProductLegacyMappings = map[string][]string{
	"listProducts":  {"LEGACY-API-0525"},
	"createProduct": {"LEGACY-API-0526"},
	"getProduct":    {"LEGACY-API-0530"},
}

var p4MediaOperations = map[string]bool{
	"uploadLegacyImage": true,
}

var p4MediaLegacyMappings = map[string][]string{
	"uploadLegacyImage": {"LEGACY-API-0361"},
}

var p4GroupInviteOperations = map[string]bool{
	"listLegacyGroupInvites": true, "createLegacyGroupInvite": true, "getLegacyGroupInvite": true,
	"updateLegacyGroupInvite": true, "archiveLegacyGroupInvite": true,
}

var p4GroupInviteLegacyMappings = map[string][]string{
	"listLegacyGroupInvites": {"LEGACY-API-0335"}, "createLegacyGroupInvite": {"LEGACY-API-0336"},
	"getLegacyGroupInvite": {"LEGACY-API-0338"}, "updateLegacyGroupInvite": {"LEGACY-API-0339"},
	"archiveLegacyGroupInvite": {"LEGACY-API-0337"},
}

var p4SurveyOperations = map[string]bool{
	"listLegacyQuestionnaires": true, "createLegacyQuestionnaire": true, "getLegacyQuestionnaire": true,
	"replaceLegacyQuestionnaire": true, "updateLegacyQuestionnaire": true,
	"deleteLegacyQuestionnaire": true, "duplicateLegacyQuestionnaire": true,
	"disableLegacyQuestionnaire": true, "enableLegacyQuestionnaire": true,
}

var p4SurveyLegacyMappings = map[string][]string{
	"listLegacyQuestionnaires":     {"LEGACY-API-0423"},
	"createLegacyQuestionnaire":    {"LEGACY-API-0424"},
	"getLegacyQuestionnaire":       {"LEGACY-API-0427"},
	"replaceLegacyQuestionnaire":   {"LEGACY-API-0429"},
	"updateLegacyQuestionnaire":    {"LEGACY-API-0428"},
	"deleteLegacyQuestionnaire":    {"LEGACY-API-0426"},
	"duplicateLegacyQuestionnaire": {"LEGACY-API-0431"},
	"disableLegacyQuestionnaire":   {"LEGACY-API-0430"},
	"enableLegacyQuestionnaire":    {"LEGACY-API-0432"},
}

var p4SurveyEvidence = map[string]string{
	"listLegacyQuestionnaires":     "P4-F01A-2026-08-15",
	"createLegacyQuestionnaire":    "P4-F01A-2026-08-15",
	"getLegacyQuestionnaire":       "P4-F01A-2026-08-15",
	"replaceLegacyQuestionnaire":   "P4-F01AB-2026-08-15",
	"updateLegacyQuestionnaire":    "P4-F01AB-2026-08-15",
	"deleteLegacyQuestionnaire":    "P4-F01AB-2026-08-15",
	"duplicateLegacyQuestionnaire": "P4-F01AB-2026-08-15",
	"disableLegacyQuestionnaire":   "P4-F01AB-2026-08-15",
	"enableLegacyQuestionnaire":    "P4-F01AB-2026-08-15",
}

var p4ChannelOperations = map[string]bool{
	"listLegacyChannels": true, "createLegacyChannel": true, "getLegacyChannel": true, "updateLegacyChannel": true,
}

var p4ChannelLegacyMappings = map[string][]string{
	"listLegacyChannels":  {"LEGACY-API-0190"},
	"createLegacyChannel": {"LEGACY-API-0191"},
	"getLegacyChannel":    {"LEGACY-API-0195"},
	"updateLegacyChannel": {"LEGACY-API-0196"},
}

var p4TagOperations = map[string]bool{
	"listLegacyWecomTags": true, "createLegacyWecomTagGroup": true,
	"updateLegacyWecomTagGroupPut": true, "updateLegacyWecomTagGroupPatch": true, "archiveLegacyWecomTagGroup": true,
	"createLegacyWecomTag": true, "updateLegacyWecomTagPut": true, "updateLegacyWecomTagPatch": true, "archiveLegacyWecomTag": true,
}

var p4TagLegacyMappings = map[string][]string{
	"listLegacyWecomTags": {"LEGACY-API-0555"}, "createLegacyWecomTagGroup": {"LEGACY-API-0552"},
	"updateLegacyWecomTagGroupPut": {"LEGACY-API-0553"}, "updateLegacyWecomTagGroupPatch": {"LEGACY-API-0553"}, "archiveLegacyWecomTagGroup": {"LEGACY-API-0553"},
	"createLegacyWecomTag": {"LEGACY-API-0556"}, "updateLegacyWecomTagPut": {"LEGACY-API-0562"}, "updateLegacyWecomTagPatch": {"LEGACY-API-0562"}, "archiveLegacyWecomTag": {"LEGACY-API-0562"},
}

var p4CouponOperations = map[string]bool{
	"listLegacyCoupons": true, "createLegacyCoupon": true, "getLegacyCoupon": true,
	"updateLegacyCoupon": true, "publishLegacyCoupon": true, "stopLegacyCoupon": true,
	"getLegacyCouponListPage": true, "getLegacyCouponNewPage": true,
	"getLegacyCouponDataPage": true, "getLegacyCouponEditPage": true,
	"listLegacyCouponProductOptions": true, "deleteLegacyCoupon": true,
	"archiveLegacyCoupon": true, "listLegacyCouponClaims": true,
	"copyLegacyCoupon": true, "getLegacyCouponShare": true,
	"listH5AvailableCoupons": true, "getH5CouponState": true,
	"claimH5Coupon": true, "listSidebarCoupons": true, "getPublicCouponPage": true,
}

var p4CouponLegacyMappings = map[string][]string{
	"listLegacyCoupons": {"LEGACY-API-0285"}, "createLegacyCoupon": {"LEGACY-API-0286"},
	"getLegacyCoupon": {"LEGACY-API-0289"}, "updateLegacyCoupon": {"LEGACY-API-0290"},
	"publishLegacyCoupon": {"LEGACY-API-0294"}, "stopLegacyCoupon": {"LEGACY-API-0296"},
	"getLegacyCouponListPage": {"LEGACY-API-0043"}, "getLegacyCouponNewPage": {"LEGACY-API-0044"},
	"getLegacyCouponDataPage": {"LEGACY-API-0045"}, "getLegacyCouponEditPage": {"LEGACY-API-0046"},
	"listLegacyCouponProductOptions": {"LEGACY-API-0287"}, "deleteLegacyCoupon": {"LEGACY-API-0288"},
	"archiveLegacyCoupon": {"LEGACY-API-0291"}, "listLegacyCouponClaims": {"LEGACY-API-0292"},
	"copyLegacyCoupon": {"LEGACY-API-0293"}, "getLegacyCouponShare": {"LEGACY-API-0295"},
	"listH5AvailableCoupons": {"LEGACY-API-0642"}, "getH5CouponState": {"LEGACY-API-0643"},
	"claimH5Coupon": {"LEGACY-API-0644"}, "listSidebarCoupons": {"LEGACY-API-0727"},
	"getPublicCouponPage": {"LEGACY-API-0756"},
}

var p4CouponEvidence = map[string]string{
	"listLegacyCoupons": p4CouponJ01DecisionEvidence, "createLegacyCoupon": p4CouponJ01DecisionEvidence,
	"getLegacyCoupon": p4CouponJ01DecisionEvidence, "updateLegacyCoupon": p4CouponJ01DecisionEvidence,
	"publishLegacyCoupon": p4CouponJ01DecisionEvidence, "stopLegacyCoupon": p4CouponJ01DecisionEvidence,
	"getLegacyCouponListPage": p4CouponABDecisionEvidence, "getLegacyCouponNewPage": p4CouponABDecisionEvidence,
	"getLegacyCouponDataPage": p4CouponABDecisionEvidence, "getLegacyCouponEditPage": p4CouponABDecisionEvidence,
	"listLegacyCouponProductOptions": p4CouponABDecisionEvidence, "deleteLegacyCoupon": p4CouponABDecisionEvidence,
	"archiveLegacyCoupon": p4CouponABDecisionEvidence, "listLegacyCouponClaims": p4CouponABDecisionEvidence,
	"copyLegacyCoupon": p4CouponABDecisionEvidence, "getLegacyCouponShare": p4CouponABDecisionEvidence,
	"listH5AvailableCoupons": p4CouponABDecisionEvidence, "getH5CouponState": p4CouponABDecisionEvidence,
	"claimH5Coupon": p4CouponABDecisionEvidence, "listSidebarCoupons": p4CouponABDecisionEvidence,
	"getPublicCouponPage": p4CouponABDecisionEvidence,
}

type couponPublicAccessContract struct {
	authScheme  string
	accessScope string
}

var couponPublicAccessContracts = map[string]couponPublicAccessContract{
	"listH5AvailableCoupons": {authScheme: "payment_identity_session", accessScope: "self"},
	"getH5CouponState":       {authScheme: "public"},
	"claimH5Coupon":          {authScheme: "payment_identity_session", accessScope: "self"},
	"listSidebarCoupons":     {authScheme: "sidebar_grant", accessScope: "owner"},
	"getPublicCouponPage":    {authScheme: "public"},
}

var p4OrderOperations = map[string]bool{"listLegacyOrders": true}

var p4OrderLegacyMappings = map[string][]string{
	"listLegacyOrders": {"LEGACY-API-0405"},
}

var p4CustomerCompatOperations = map[string]bool{
	"listLegacyCustomers": true, "getLegacyCustomer": true,
}

var p4CustomerCompatLegacyMappings = map[string][]string{
	"listLegacyCustomers": {"LEGACY-API-0609"},
	"getLegacyCustomer":   {"LEGACY-API-0619"},
}

var p4ConfigSettingsOperations = map[string]bool{
	"getLegacyAppSettingsPage": true, "saveLegacyAppSettingsPage": true, "getLegacyAppSettingsResource": true,
}

var p4ConfigSettingsLegacyMappings = map[string][]string{
	"getLegacyAppSettingsPage": {"LEGACY-API-0026"}, "saveLegacyAppSettingsPage": {"LEGACY-API-0027"},
	"getLegacyAppSettingsResource": {"LEGACY-API-0253"},
}

var identityOperations = map[string]bool{
	"resolveIdentity": true, "bindIdentity": true, "ingestIdentityEvent": true,
	"listIdentityMergeReviews": true, "approveIdentityMergeReview": true,
	"rejectIdentityMergeReview": true,
}

var contactOperations = map[string]bool{
	"listCustomers": true, "getCustomer": true, "updateCustomer": true,
	"listCustomerEvents": true, "listTags": true, "setCustomerStage": true,
	"addCustomerTag": true, "removeCustomerTag": true,
}

var segmentOperations = map[string]bool{
	"listSegments": true, "createSegment": true, "getSegment": true,
	"updateSegment": true, "listSegmentMembers": true, "requestSegmentRefresh": true,
}

type authorizationContract struct {
	capability string
	scopes     map[string]string
}

var authorizationContracts = map[string]authorizationContract{
	"listCustomers":                  {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"getCustomer":                    {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"updateCustomer":                 {"customers.write", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"listCustomerEvents":             {"customer.events.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"resolveIdentity":                {"identity.resolve", map[string]string{"admin": "global", "ops": "global"}},
	"bindIdentity":                   {"identity.bind", map[string]string{"admin": "global", "ops": "global"}},
	"ingestIdentityEvent":            {"identity.ingest", map[string]string{"admin": "global", "ops": "global"}},
	"listIdentityMergeReviews":       {"identity.review.read", map[string]string{"admin": "global", "ops": "global"}},
	"approveIdentityMergeReview":     {"identity.review.write", map[string]string{"admin": "global", "ops": "global"}},
	"rejectIdentityMergeReview":      {"identity.review.write", map[string]string{"admin": "global", "ops": "global"}},
	"getAuthSession":                 {"auth.session.read", map[string]string{"admin": "self", "ops": "self", "sales": "self"}},
	"logoutAdmin":                    {"auth.session.logout", map[string]string{"admin": "self", "ops": "self", "sales": "self"}},
	"getAdminConfigOverview":         {"config.overview.read", map[string]string{"admin": "global"}},
	"listStages":                     {"stages.read", map[string]string{"admin": "global", "ops": "global", "sales": "global"}},
	"createStage":                    {"stages.write", map[string]string{"admin": "global", "ops": "global"}},
	"renameStage":                    {"stages.write", map[string]string{"admin": "global", "ops": "global"}},
	"listTags":                       {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"setCustomerStage":               {"customers.write", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"addCustomerTag":                 {"customers.write", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"removeCustomerTag":              {"customers.write", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"listSegments":                   {"segments.read", map[string]string{"admin": "global", "ops": "global"}},
	"getSegment":                     {"segments.read", map[string]string{"admin": "global", "ops": "global"}},
	"listSegmentMembers":             {"segments.read", map[string]string{"admin": "global", "ops": "global"}},
	"createSegment":                  {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateSegment":                  {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"requestSegmentRefresh":          {"segments.write", map[string]string{"admin": "global", "ops": "global"}},
	"listAutomationTriggerRuns":      {"config.overview.read", map[string]string{"admin": "global"}},
	"listProducts":                   {"products.read", map[string]string{"admin": "global", "ops": "global"}},
	"createProduct":                  {"products.write", map[string]string{"admin": "global", "ops": "global"}},
	"getProduct":                     {"products.read", map[string]string{"admin": "global", "ops": "global"}},
	"uploadLegacyImage":              {"media.images.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyGroupInvites":         {"media.library.read", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyGroupInvite":        {"media.library.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyGroupInvite":           {"media.library.read", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyGroupInvite":        {"media.library.write", map[string]string{"admin": "global", "ops": "global"}},
	"archiveLegacyGroupInvite":       {"media.library.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyQuestionnaires":       {"questionnaires.read", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyQuestionnaire":      {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyQuestionnaire":         {"questionnaires.read", map[string]string{"admin": "global", "ops": "global"}},
	"replaceLegacyQuestionnaire":     {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyQuestionnaire":      {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"deleteLegacyQuestionnaire":      {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"duplicateLegacyQuestionnaire":   {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"disableLegacyQuestionnaire":     {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"enableLegacyQuestionnaire":      {"questionnaires.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyChannels":             {"channels.read", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyChannel":            {"channels.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyChannel":               {"channels.read", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyChannel":            {"channels.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyWecomTags":            {"customers.read", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyWecomTagGroup":      {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyWecomTagGroupPut":   {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyWecomTagGroupPatch": {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"archiveLegacyWecomTagGroup":     {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyWecomTag":           {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyWecomTagPut":        {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyWecomTagPatch":      {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"archiveLegacyWecomTag":          {"customers.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyCoupons":              {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"createLegacyCoupon":             {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyCoupon":                {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"updateLegacyCoupon":             {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"publishLegacyCoupon":            {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"stopLegacyCoupon":               {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyCouponListPage":        {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyCouponNewPage":         {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyCouponDataPage":        {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyCouponEditPage":        {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyCouponProductOptions": {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"deleteLegacyCoupon":             {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"archiveLegacyCoupon":            {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyCouponClaims":         {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"copyLegacyCoupon":               {"coupons.write", map[string]string{"admin": "global", "ops": "global"}},
	"getLegacyCouponShare":           {"coupons.read", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyOrders":               {"order.read", map[string]string{"admin": "global", "ops": "global"}},
	"listLegacyCustomers":            {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"getLegacyCustomer":              {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"getLegacyAppSettingsPage":       {"config.settings.manage", map[string]string{"admin": "global"}},
	"saveLegacyAppSettingsPage":      {"config.settings.manage", map[string]string{"admin": "global"}},
	"getLegacyAppSettingsResource":   {"config.settings.manage", map[string]string{"admin": "global"}},
}

const g1DecisionEvidence = "G1-D01-2026-08-10"
const p2StageDecisionEvidence = "P2-16-2026-08-11"
const p3ContactDecisionEvidence = "P3-C00-2026-08-12"
const p3IdentityDecisionEvidence = "P3-I00-2026-08-12"
const p3SegmentDecisionEvidence = "P3-S00-2026-08-12"
const p4AutomationDecisionEvidence = "P4-W0-D01-2026-08-14"
const p4ProductDecisionEvidence = "P4-I01A-2026-08-14"
const p4MediaDecisionEvidence = "P4-H01A1-2026-08-14"
const p4GroupInviteDecisionEvidence = "P4-H03-2026-08-15"
const p4ChannelDecisionEvidence = "P4-C01-2026-08-15"
const p4TagDecisionEvidence = "P4-B02-2026-08-15"
const p4CouponJ01DecisionEvidence = "P4-J01-2026-08-15"
const p4CouponABDecisionEvidence = "P4-COUPON-AB-2026-08-15"
const p4OrderDecisionEvidence = "P4-I03-2026-08-15"
const p4CustomerCompatDecisionEvidence = "P4-B01-2026-08-15"
const p4ConfigSettingsDecisionEvidence = "P4-A02-2026-08-15"

func main() {
	spec := flag.String("spec", "../api/openapi.yaml", "OpenAPI document")
	mapping := flag.String("mapping", "../docs/api-mapping.jsonl", "legacy API mapping")
	flag.Parse()
	doc, ids, err := load(*spec, *mapping)
	if err == nil {
		err = validate(doc, ids)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "openapi-contract:", err)
		os.Exit(1)
	}
	fmt.Println("openapi-contract: PASS (p1_operations=10 approved=10 legacy_links=70 p2_stage_operations=3 p3_contact_operations=4 p3_identity_operations=3 p3_segment_operations=6 p4_automation_operations=1 p4_product_operations=3 p4_media_operations=1 p4_group_invite_operations=5 p4_survey_operations=9 p4_channel_operations=4 p4_coupon_operations=21 p4_order_operations=1 p4_customer_compat_operations=2 p4_config_settings_operations=3)")
}

func load(spec, mapping string) (*openapi3.T, map[string]bool, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromFile(spec)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(mapping)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	ids := map[string]bool{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var row struct {
			MappingID string `json:"mapping_id"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, nil, err
		}
		if row.MappingID == "" || ids[row.MappingID] {
			return nil, nil, errors.New("invalid legacy mapping IDs")
		}
		ids[row.MappingID] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	if len(ids) != 781 {
		return nil, nil, fmt.Errorf("legacy mapping inventory=%d", len(ids))
	}
	return doc, ids, nil
}

func validate(doc *openapi3.T, known map[string]bool) error {
	if err := doc.Validate(context.Background()); err != nil {
		return err
	}
	if len(doc.Security) == 0 {
		return errors.New("business API lacks default security")
	}
	seenP1, seenP2 := map[string]bool{}, map[string]bool{}
	seenP3Contact, seenP3Identity, seenP3Segment, links := map[string]bool{}, map[string]bool{}, map[string]bool{}, 0
	seenP4Automation, seenP4Product, seenP4Media, seenP4GroupInvite, seenP4Survey, seenP4Channel, seenP4Tag, seenP4Coupon, seenP4Order, seenP4CustomerCompat, seenP4ConfigSettings := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for _, op := range item.Operations() {
			if path == "/healthz" {
				continue
			}
			if seenP1[op.OperationID] || seenP2[op.OperationID] || seenP3Contact[op.OperationID] || seenP3Identity[op.OperationID] || seenP3Segment[op.OperationID] || seenP4Automation[op.OperationID] || seenP4Product[op.OperationID] || seenP4Media[op.OperationID] || seenP4GroupInvite[op.OperationID] || seenP4Survey[op.OperationID] || seenP4Channel[op.OperationID] || seenP4Tag[op.OperationID] || seenP4Coupon[op.OperationID] || seenP4Order[op.OperationID] || seenP4CustomerCompat[op.OperationID] || seenP4ConfigSettings[op.OperationID] ||
				(!p1CandidateOperations[op.OperationID] && !p2StageOperations[op.OperationID] &&
					!p3ContactOperations[op.OperationID] && !p3IdentityOperations[op.OperationID] && !p3SegmentOperations[op.OperationID] && !p4AutomationOperations[op.OperationID] && !p4ProductOperations[op.OperationID] && !p4MediaOperations[op.OperationID] && !p4GroupInviteOperations[op.OperationID] && !p4SurveyOperations[op.OperationID] && !p4ChannelOperations[op.OperationID] && !p4TagOperations[op.OperationID] && !p4CouponOperations[op.OperationID] && !p4OrderOperations[op.OperationID] && !p4CustomerCompatOperations[op.OperationID] && !p4ConfigSettingsOperations[op.OperationID]) {
				return fmt.Errorf("unexpected or duplicate candidate operation: %s", op.OperationID)
			}
			if p1CandidateOperations[op.OperationID] {
				seenP1[op.OperationID] = true
				status, ok := op.Extensions["x-p1-signoff-status"].(string)
				if !ok || status != "APPROVED" {
					return fmt.Errorf("%s lacks approved G1 signoff", op.OperationID)
				}
				evidence, ok := op.Extensions["x-p1-decision-evidence"].(string)
				if !ok || evidence != g1DecisionEvidence {
					return fmt.Errorf("%s has missing or forged G1 evidence", op.OperationID)
				}
				ids, err := stringList(op.Extensions["x-legacy-mapping-ids"])
				if err != nil || len(ids) == 0 {
					return fmt.Errorf("%s lacks legacy links", op.OperationID)
				}
				for _, id := range ids {
					if !known[id] {
						return fmt.Errorf("%s links unknown mapping %s", op.OperationID, id)
					}
					links++
				}
			} else if p2StageOperations[op.OperationID] {
				seenP2[op.OperationID] = true
				evidence, ok := op.Extensions["x-p2-decision-evidence"].(string)
				if !ok || evidence != p2StageDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P2 stage evidence", op.OperationID)
				}
			} else if p3ContactOperations[op.OperationID] {
				seenP3Contact[op.OperationID] = true
				if ids, ok := op.Extensions["x-legacy-mapping-ids"]; ok {
					legacyIDs, err := stringList(ids)
					if err != nil || len(legacyIDs) == 0 {
						return fmt.Errorf("%s has invalid legacy links", op.OperationID)
					}
					for _, id := range legacyIDs {
						if !known[id] {
							return fmt.Errorf("%s links unknown mapping %s", op.OperationID, id)
						}
						links++
					}
				}
			} else if p3IdentityOperations[op.OperationID] {
				seenP3Identity[op.OperationID] = true
			} else if p3SegmentOperations[op.OperationID] {
				seenP3Segment[op.OperationID] = true
			} else if p4AutomationOperations[op.OperationID] {
				seenP4Automation[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4AutomationDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Automation evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, []string{"LEGACY-API-0141"}) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4ProductOperations[op.OperationID] {
				seenP4Product[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4ProductDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Product evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4ProductLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4MediaOperations[op.OperationID] {
				seenP4Media[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4MediaDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Media evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4MediaLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
			} else if p4GroupInviteOperations[op.OperationID] {
				seenP4GroupInvite[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4GroupInviteDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Group Invite evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4GroupInviteLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				links++
			} else if p4SurveyOperations[op.OperationID] {
				seenP4Survey[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4SurveyEvidence[op.OperationID] {
					return fmt.Errorf("%s has missing or forged P4 Survey evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4SurveyLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				links++
			} else if p4ChannelOperations[op.OperationID] {
				seenP4Channel[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4ChannelDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Channel evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4ChannelLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				links++
			} else if p4TagOperations[op.OperationID] {
				seenP4Tag[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4TagDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Tag evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4TagLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				links++
			} else if p4CouponOperations[op.OperationID] {
				seenP4Coupon[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4CouponEvidence[op.OperationID] {
					return fmt.Errorf("%s has missing or forged P4 Coupon evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4CouponLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				links++
			} else if p4OrderOperations[op.OperationID] {
				seenP4Order[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4OrderDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Order evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4OrderLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				links++
			} else if p4CustomerCompatOperations[op.OperationID] {
				seenP4CustomerCompat[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4CustomerCompatDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Customer evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4CustomerCompatLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				links++
			} else {
				seenP4ConfigSettings[op.OperationID] = true
				evidence, ok := op.Extensions["x-p4-decision-evidence"].(string)
				if !ok || evidence != p4ConfigSettingsDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P4 Config Settings evidence", op.OperationID)
				}
				ids, linkErr := stringList(op.Extensions["x-legacy-mapping-ids"])
				if linkErr != nil || !reflect.DeepEqual(ids, p4ConfigSettingsLegacyMappings[op.OperationID]) {
					return fmt.Errorf("%s legacy mapping=%v", op.OperationID, ids)
				}
				links++
			}
			if contactOperations[op.OperationID] {
				evidence, ok := op.Extensions["x-p3-decision-evidence"].(string)
				if !ok || evidence != p3ContactDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P3 contact evidence", op.OperationID)
				}
			}
			if identityOperations[op.OperationID] {
				evidence, ok := op.Extensions["x-p3-decision-evidence"].(string)
				if !ok || evidence != p3IdentityDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P3 identity evidence", op.OperationID)
				}
			}
			if segmentOperations[op.OperationID] {
				evidence, ok := op.Extensions["x-p3-decision-evidence"].(string)
				if !ok || evidence != p3SegmentDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P3 segment evidence", op.OperationID)
				}
			}
			if public, publicOperation := couponPublicAccessContracts[op.OperationID]; publicOperation {
				authScheme, ok := op.Extensions["x-aicrm-auth-scheme"].(string)
				if !ok || authScheme != public.authScheme {
					return fmt.Errorf("%s public auth scheme=%q", op.OperationID, authScheme)
				}
				if public.accessScope != "" {
					accessScope, ok := op.Extensions["x-aicrm-access-scope"].(string)
					if !ok || accessScope != public.accessScope {
						return fmt.Errorf("%s public access scope=%q", op.OperationID, accessScope)
					}
				}
			} else {
				contract, found := authorizationContracts[op.OperationID]
				if !found {
					return fmt.Errorf("%s lacks authorization contract", op.OperationID)
				}
				capability, ok := op.Extensions["x-aicrm-capability"].(string)
				if !ok || capability != contract.capability {
					return fmt.Errorf("%s capability=%q", op.OperationID, capability)
				}
				scopes, err := stringMap(op.Extensions["x-aicrm-rbac-scopes"])
				if err != nil || !reflect.DeepEqual(scopes, contract.scopes) {
					return fmt.Errorf("%s RBAC scopes=%v", op.OperationID, scopes)
				}
				if len(contract.scopes) < 3 && op.Responses.Value("403") == nil {
					return fmt.Errorf("%s denies a role but lacks 403", op.OperationID)
				}
			}
			if op.OperationID == "listTags" && op.Responses.Value("503") == nil {
				return fmt.Errorf("listTags lacks dependency unavailable response")
			}
		}
	}
	if len(seenP1) != 10 || len(seenP2) != 3 || len(seenP3Contact) != 4 || len(seenP3Identity) != 3 || len(seenP3Segment) != 6 || len(seenP4Automation) != 1 || len(seenP4Product) != 3 || len(seenP4Media) != 1 || len(seenP4GroupInvite) != 5 || len(seenP4Survey) != 9 || len(seenP4Channel) != 4 || len(seenP4Tag) != 9 || len(seenP4Coupon) != 21 || len(seenP4Order) != 1 || len(seenP4CustomerCompat) != 2 || len(seenP4ConfigSettings) != 3 || links != 70 {
		return fmt.Errorf("candidate inventory mismatch: p1=%d p2_stages=%d p3_contact=%d p3_identity=%d p3_segment=%d p4_automation=%d p4_product=%d p4_media=%d p4_survey=%d p4_channel=%d p4_coupon=%d p4_order=%d p4_customer_compat=%d links=%d", len(seenP1), len(seenP2), len(seenP3Contact), len(seenP3Identity), len(seenP3Segment), len(seenP4Automation), len(seenP4Product), len(seenP4Media), len(seenP4Survey), len(seenP4Channel), len(seenP4Coupon), len(seenP4Order), len(seenP4CustomerCompat), links)
	}
	for id := range p1CandidateOperations {
		if !seenP1[id] {
			return fmt.Errorf("missing candidate operation: %s", id)
		}
	}
	for id := range p2StageOperations {
		if !seenP2[id] {
			return fmt.Errorf("missing P2 stage operation: %s", id)
		}
	}
	for id := range p3ContactOperations {
		if !seenP3Contact[id] {
			return fmt.Errorf("missing P3 contact operation: %s", id)
		}
	}
	for id := range p3IdentityOperations {
		if !seenP3Identity[id] {
			return fmt.Errorf("missing P3 identity operation: %s", id)
		}
	}
	for id := range p3SegmentOperations {
		if !seenP3Segment[id] {
			return fmt.Errorf("missing P3 segment operation: %s", id)
		}
	}
	for id := range p4AutomationOperations {
		if !seenP4Automation[id] {
			return fmt.Errorf("missing P4 Automation operation: %s", id)
		}
	}
	for id := range p4ProductOperations {
		if !seenP4Product[id] {
			return fmt.Errorf("missing P4 Product operation: %s", id)
		}
	}
	for id := range p4MediaOperations {
		if !seenP4Media[id] {
			return fmt.Errorf("missing P4 Media operation: %s", id)
		}
	}
	for id := range p4GroupInviteOperations {
		if !seenP4GroupInvite[id] {
			return fmt.Errorf("missing P4 Group Invite operation: %s", id)
		}
	}
	for id := range p4SurveyOperations {
		if !seenP4Survey[id] {
			return fmt.Errorf("missing P4 Survey operation: %s", id)
		}
	}
	for id := range p4ChannelOperations {
		if !seenP4Channel[id] {
			return fmt.Errorf("missing P4 Channel operation: %s", id)
		}
	}
	for id := range p4TagOperations {
		if !seenP4Tag[id] {
			return fmt.Errorf("missing P4 Tag operation: %s", id)
		}
	}
	for id := range p4CouponOperations {
		if !seenP4Coupon[id] {
			return fmt.Errorf("missing P4 Coupon operation: %s", id)
		}
	}
	for id := range p4OrderOperations {
		if !seenP4Order[id] {
			return fmt.Errorf("missing P4 Order operation: %s", id)
		}
	}
	for id := range p4CustomerCompatOperations {
		if !seenP4CustomerCompat[id] {
			return fmt.Errorf("missing P4 Customer compatibility operation: %s", id)
		}
	}
	for id := range p4ConfigSettingsOperations {
		if !seenP4ConfigSettings[id] {
			return fmt.Errorf("missing P4 Config Settings compatibility operation: %s", id)
		}
	}
	customer := doc.Components.Schemas["Customer"]
	if customer == nil || customer.Value == nil {
		return errors.New("Customer schema missing")
	}
	for _, name := range []string{"external_userid", "unionid", "openid", "phone", "mobile"} {
		if _, ok := customer.Value.Properties[name]; ok {
			return fmt.Errorf("Customer contains external identity: %s", name)
		}
	}
	identity := doc.Components.Schemas["IdentityRef"]
	if identity == nil || identity.Value == nil {
		return errors.New("IdentityRef schema missing")
	}
	required := append([]string(nil), identity.Value.Required...)
	sort.Strings(required)
	want := []string{"scope", "type", "value"}
	if fmt.Sprint(required) != fmt.Sprint(want) {
		return fmt.Errorf("IdentityRef required fields=%v", required)
	}
	if len(identity.Value.Properties) != 3 || identity.Value.AdditionalProperties.Has == nil ||
		*identity.Value.AdditionalProperties.Has {
		return errors.New("IdentityRef must be a closed type/scope/value admin shape")
	}
	kinds, err := stringList(identity.Value.Properties["type"].Value.Enum)
	if err != nil {
		return errors.New("IdentityRef type enum is invalid")
	}
	sort.Strings(kinds)
	wantKinds := []string{"alipay_user_id", "ext", "mp_openid", "oa_openid", "phone", "unionid", "wecom_external_userid"}
	if !reflect.DeepEqual(kinds, wantKinds) {
		return fmt.Errorf("IdentityRef type enum=%v", kinds)
	}
	if doc.Components.Schemas["ErrorResponse"] == nil {
		return errors.New("ErrorResponse schema missing")
	}
	if err := validateBrowserSessionContract(doc); err != nil {
		return err
	}
	if err := validateStageContract(doc); err != nil {
		return err
	}
	if err := validateContactContract(doc); err != nil {
		return err
	}
	if err := validateIdentityContract(doc); err != nil {
		return err
	}
	if err := validateSegmentContract(doc); err != nil {
		return err
	}
	if err := validateAutomationContract(doc); err != nil {
		return err
	}
	if err := validateGroupInviteContract(doc); err != nil {
		return err
	}
	if err := validateSurveyContract(doc); err != nil {
		return err
	}
	if err := validateChannelContract(doc); err != nil {
		return err
	}
	if err := validateCouponContract(doc); err != nil {
		return err
	}
	if err := validateCustomerCompatContract(doc); err != nil {
		return err
	}
	if err := validateConfigSettingsContract(doc); err != nil {
		return err
	}
	return nil
}

func validateConfigSettingsContract(doc *openapi3.T) error {
	page := doc.Paths.Value("/admin/config/app-settings")
	save := doc.Paths.Value("/admin/config/app-settings/save")
	resource := doc.Paths.Value("/api/admin/config/app-settings")
	if page == nil || page.Get == nil || save == nil || save.Post == nil || resource == nil || resource.Get == nil {
		return errors.New("P4-A02 Config Settings compatibility operations are incomplete")
	}
	if !operationResponseUsesLocalSchema(resource.Get, "LegacyAppSettingsResponse") || !operationFormUsesLocalSchema(save.Post, "LegacyAppSettingsSaveForm") {
		return errors.New("P4-A02 Config Settings form or resource schema drifted")
	}
	form := doc.Components.Schemas["LegacyAppSettingsSaveForm"]
	masked := doc.Components.Schemas["LegacyMaskedAppSetting"]
	editable := doc.Components.Schemas["LegacyEditableAppSetting"]
	if form == nil || form.Value == nil || form.Value.AdditionalProperties.Has == nil || *form.Value.AdditionalProperties.Has || len(form.Value.Properties) != 16 {
		return errors.New("LegacyAppSettingsSaveForm must remain closed with four transport fields plus twelve settings")
	}
	if masked == nil || masked.Value == nil || masked.Value.AdditionalProperties.Has == nil || *masked.Value.AdditionalProperties.Has || len(masked.Value.Properties) != 7 {
		return errors.New("LegacyMaskedAppSetting must remain boolean-only")
	}
	if editable == nil || editable.Value == nil || editable.Value.Properties["version"] == nil || editable.Value.Properties["version"].Value == nil || len(editable.Value.Properties["version"].Value.Enum) != 1 || editable.Value.Properties["version"].Value.Enum[0] != "" {
		return errors.New("LegacyEditableAppSetting version must remain fixed empty")
	}
	for _, key := range []string{"database.url", "wecom.secret", "wecom.callback_token", "wecom.callback_aes_key", "ai.api_key", "auth.jwt_secret", "extension.api_key_pepper", "gateway.webhook_master_key"} {
		property := form.Value.Properties["setting__"+key]
		if property == nil || property.Value == nil || property.Value.MaxLength == nil || *property.Value.MaxLength != 0 {
			return fmt.Errorf("secret form key %s must only accept blank preservation", key)
		}
	}
	if save.Post.Responses.Value("302") == nil || save.Post.Responses.Value("200") != nil {
		return errors.New("P4-A02 save redirect contract drifted")
	}
	if value, ok := save.Post.Extensions["x-aicrm-session-bound-csrf"].(string); !ok || value != "required" {
		return errors.New("P4-A02 session-bound CSRF requirement drifted")
	}
	return nil
}

func operationFormUsesLocalSchema(operation *openapi3.Operation, schemaName string) bool {
	if operation == nil || operation.RequestBody == nil || operation.RequestBody.Value == nil {
		return false
	}
	media := operation.RequestBody.Value.Content["application/x-www-form-urlencoded"]
	return media != nil && media.Schema != nil && media.Schema.Ref == "#/components/schemas/"+schemaName
}

func validateGroupInviteContract(doc *openapi3.T) error {
	collection := doc.Paths.Value("/api/admin/group-invite-library")
	detail := doc.Paths.Value("/api/admin/group-invite-library/{item_id}")
	if collection == nil || collection.Get == nil || collection.Post == nil || detail == nil || detail.Get == nil || detail.Put == nil || detail.Delete == nil {
		return errors.New("P4-H03 Group Invite compatibility operations are incomplete")
	}
	if !operationResponseUsesLocalSchema(collection.Get, "LegacyGroupInviteListResponse") ||
		!operationRequestUsesLocalSchema(collection.Post, "LegacyGroupInviteCreateRequest") ||
		!operationResponseUsesLocalSchema(collection.Post, "LegacyGroupInviteMutationResponse") ||
		!operationResponseUsesLocalSchema(detail.Get, "LegacyGroupInviteDetailResponse") ||
		!operationRequestUsesLocalSchema(detail.Put, "LegacyGroupInviteUpdateRequest") ||
		!operationResponseUsesLocalSchema(detail.Put, "LegacyGroupInviteMutationResponse") ||
		!operationResponseUsesLocalSchema(detail.Delete, "LegacyGroupInviteArchiveResponse") {
		return errors.New("P4-H03 Group Invite request or response schema drifted")
	}
	for name, operation := range map[string]*openapi3.Operation{"createLegacyGroupInvite": collection.Post, "updateLegacyGroupInvite": detail.Put, "archiveLegacyGroupInvite": detail.Delete} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if operation.Responses.Value("409") == nil || operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s conflict or dependency response drifted", name)
		}
	}
	return nil
}

func validateChannelContract(doc *openapi3.T) error {
	collection := doc.Paths.Value("/api/admin/channels")
	detail := doc.Paths.Value("/api/admin/channels/{channel_id}")
	if collection == nil || collection.Get == nil || collection.Post == nil || detail == nil || detail.Get == nil || detail.Patch == nil {
		return errors.New("P4-C01 Channel compatibility operations are incomplete")
	}
	if !operationResponseUsesLocalSchema(collection.Get, "LegacyChannelListResponse") ||
		!operationRequestUsesLocalSchema(collection.Post, "LegacyChannelWriteRequest") ||
		!operationResponseUsesStatusLocalSchema(collection.Post, "201", "LegacyChannelMutationResponse") ||
		!operationResponseUsesLocalSchema(detail.Get, "LegacyChannelDetailResponse") ||
		!operationRequestUsesLocalSchema(detail.Patch, "LegacyChannelWriteRequest") ||
		!operationResponseUsesLocalSchema(detail.Patch, "LegacyChannelMutationResponse") {
		return errors.New("P4-C01 Channel request or response schema drifted")
	}
	for name, operation := range map[string]*openapi3.Operation{"createLegacyChannel": collection.Post, "updateLegacyChannel": detail.Patch} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if operation.Responses.Value("400") == nil || operation.Responses.Value("409") == nil || operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s boundary or failure responses drifted", name)
		}
	}
	request := doc.Components.Schemas["LegacyChannelWriteRequest"]
	if request == nil || request.Value == nil || request.Value.AdditionalProperties.Has == nil || *request.Value.AdditionalProperties.Has {
		return errors.New("LegacyChannelWriteRequest must remain closed")
	}
	return nil
}

func validateCouponContract(doc *openapi3.T) error {
	collection := doc.Paths.Value("/api/admin/coupons")
	detail := doc.Paths.Value("/api/admin/coupons/{coupon_id}")
	publish := doc.Paths.Value("/api/admin/coupons/{coupon_id}/publish")
	stop := doc.Paths.Value("/api/admin/coupons/{coupon_id}/stop")
	listPage := doc.Paths.Value("/admin/coupons")
	newPage := doc.Paths.Value("/admin/coupons/new")
	dataPage := doc.Paths.Value("/admin/coupons/{coupon_id}/data")
	editPage := doc.Paths.Value("/admin/coupons/{coupon_id}/edit")
	options := doc.Paths.Value("/api/admin/coupons/product-options")
	archive := doc.Paths.Value("/api/admin/coupons/{coupon_id}/archive")
	claims := doc.Paths.Value("/api/admin/coupons/{coupon_id}/claims")
	copyPage := doc.Paths.Value("/api/admin/coupons/{coupon_id}/copy")
	share := doc.Paths.Value("/api/admin/coupons/{coupon_id}/share")
	h5Available := doc.Paths.Value("/api/h5/coupons/available")
	h5Coupon := doc.Paths.Value("/api/h5/coupons/{public_slug}")
	h5Claim := doc.Paths.Value("/api/h5/coupons/{public_slug}/claim")
	sidebar := doc.Paths.Value("/api/sidebar/v2/coupons")
	publicPage := doc.Paths.Value("/c/{public_slug}")
	if collection == nil || collection.Get == nil || collection.Post == nil || detail == nil || detail.Get == nil || detail.Put == nil || detail.Delete == nil || publish == nil || publish.Post == nil || stop == nil || stop.Post == nil ||
		listPage == nil || listPage.Get == nil || newPage == nil || newPage.Get == nil || dataPage == nil || dataPage.Get == nil || editPage == nil || editPage.Get == nil || options == nil || options.Get == nil || archive == nil || archive.Post == nil || claims == nil || claims.Get == nil || copyPage == nil || copyPage.Post == nil || share == nil || share.Get == nil || h5Available == nil || h5Available.Get == nil || h5Coupon == nil || h5Coupon.Get == nil || h5Claim == nil || h5Claim.Post == nil || sidebar == nil || sidebar.Get == nil || publicPage == nil || publicPage.Get == nil {
		return errors.New("P4 Coupon A+B compatibility operations are incomplete")
	}
	if !operationResponseUsesLocalSchema(collection.Get, "LegacyCouponListResponse") || !operationRequestUsesLocalSchema(collection.Post, "CouponUpsertRequest") || !operationResponseUsesLocalSchema(collection.Post, "LegacyCouponMutationResponse") || !operationResponseUsesLocalSchema(detail.Get, "LegacyCouponDetailResponse") || !operationRequestUsesLocalSchema(detail.Put, "CouponUpsertRequest") || !operationResponseUsesLocalSchema(detail.Put, "LegacyCouponMutationResponse") || !operationResponseUsesLocalSchema(publish.Post, "LegacyCouponMutationResponse") || !operationResponseUsesLocalSchema(stop.Post, "LegacyCouponMutationResponse") ||
		!operationResponseUsesLocalSchema(detail.Delete, "LegacyCouponBoardMutationResponse") || !operationResponseUsesLocalSchema(options.Get, "LegacyCouponProductOptionsResponse") || !operationResponseUsesLocalSchema(archive.Post, "LegacyCouponBoardMutationResponse") || !operationResponseUsesLocalSchema(claims.Get, "LegacyCouponClaimListResponse") || !operationResponseUsesLocalSchema(copyPage.Post, "LegacyCouponBoardMutationResponse") || !operationResponseUsesLocalSchema(share.Get, "LegacyCouponShareResponse") || !operationResponseUsesLocalSchema(h5Available.Get, "H5CouponAvailableResponse") || !operationResponseUsesLocalSchema(h5Coupon.Get, "H5CouponDetailResponse") || !operationResponseUsesLocalSchema(h5Claim.Post, "H5CouponClaimResponse") || !operationResponseUsesLocalSchema(sidebar.Get, "SidebarCouponListResponse") {
		return errors.New("P4 Coupon A+B request or response schema drifted")
	}
	for name, operation := range map[string]*openapi3.Operation{"createLegacyCoupon": collection.Post, "updateLegacyCoupon": detail.Put, "publishLegacyCoupon": publish.Post, "stopLegacyCoupon": stop.Post} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if operation.Responses.Value("400") == nil || operation.Responses.Value("409") == nil || operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s boundary or failure responses drifted", name)
		}
	}
	for name, operation := range map[string]*openapi3.Operation{"deleteLegacyCoupon": detail.Delete, "archiveLegacyCoupon": archive.Post, "copyLegacyCoupon": copyPage.Post} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if !hasRequiredHeader(operation, "Idempotency-Key") || operation.Responses.Value("409") == nil || operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s lacks required idempotency, conflict, or unavailable response", name)
		}
	}
	if !hasRequiredHeader(h5Claim.Post, "Idempotency-Key") || h5Claim.Post.Responses.Value("401") == nil || h5Claim.Post.Responses.Value("409") == nil || h5Claim.Post.Responses.Value("503") == nil {
		return errors.New("claimH5Coupon lacks frozen auth, idempotency, conflict, or unavailable response")
	}
	if csrf, ok := h5Claim.Post.Extensions["x-aicrm-csrf"].(string); !ok || csrf != "same_origin_empty_body" {
		return errors.New("claimH5Coupon CSRF applicability drifted")
	}
	if h5Available.Get.Responses.Value("401") == nil || sidebar.Get.Responses.Value("401") == nil || h5Coupon.Get.Responses.Value("404") == nil || publicPage.Get.Responses.Value("404") == nil {
		return errors.New("P4 Coupon public access failures drifted")
	}
	for _, parameter := range collection.Post.Parameters {
		if parameter != nil && parameter.Value != nil && parameter.Value.Name == "Idempotency-Key" {
			return errors.New("legacy coupon create must not claim replay safety")
		}
	}
	request := doc.Components.Schemas["CouponUpsertRequest"]
	if request == nil || request.Value == nil || request.Value.AdditionalProperties.Has == nil || *request.Value.AdditionalProperties.Has || len(request.Value.Properties) != 12 {
		return errors.New("CouponUpsertRequest must remain closed with 11 rule fields plus target_refs")
	}
	required := append([]string(nil), request.Value.Required...)
	sort.Strings(required)
	want := []string{"claim_ends_at", "claim_starts_at", "discount_amount_total", "name", "target_refs", "total_issue_limit", "validity_mode"}
	if !reflect.DeepEqual(required, want) {
		return fmt.Errorf("CouponUpsertRequest required=%v", required)
	}
	perUser, instructions := request.Value.Properties["per_user_issue_limit"], request.Value.Properties["instructions"]
	if perUser == nil || perUser.Value == nil || fmt.Sprint(perUser.Value.Default) != "1" || instructions == nil || instructions.Value == nil || fmt.Sprint(instructions.Value.Default) != "" {
		return errors.New("CouponUpsertRequest defaults drifted")
	}
	for _, name := range []string{"LegacyCouponProductOptionsResponse", "LegacyCouponClaimListResponse", "LegacyCouponShareResponse", "H5CouponAvailableResponse", "H5CouponDetailResponse", "H5CouponClaimResponse", "SidebarCouponListResponse"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.AdditionalProperties.Has == nil || *schema.Value.AdditionalProperties.Has {
			return fmt.Errorf("%s must remain closed", name)
		}
	}
	claim := doc.Components.Schemas["LegacyCouponClaim"]
	sidebarCoupon := doc.Components.Schemas["SidebarCoupon"]
	if claim == nil || claim.Value == nil || claim.Value.Properties["customer_id"] != nil || sidebarCoupon == nil || sidebarCoupon.Value == nil || sidebarCoupon.Value.Properties["customer_id"] != nil {
		return errors.New("Coupon claims and sidebar projections must not disclose customer IDs")
	}
	return nil
}

func validateCustomerCompatContract(doc *openapi3.T) error {
	list := doc.Paths.Value("/api/customers")
	detail := doc.Paths.Value("/api/customers/{external_userid}")
	if list == nil || list.Get == nil || detail == nil || detail.Get == nil ||
		!operationResponseUsesLocalSchema(list.Get, "LegacyCustomerListResponse") ||
		!operationResponseUsesLocalSchema(detail.Get, "LegacyCustomerDetailResponse") {
		return errors.New("P4-B01 Customer compatibility operations are incomplete")
	}
	if list.Get.Responses.Value("400") == nil || list.Get.Responses.Value("503") == nil ||
		detail.Get.Responses.Value("404") == nil || detail.Get.Responses.Value("503") == nil {
		return errors.New("P4-B01 Customer boundary or failure responses drifted")
	}
	return nil
}

func validateSurveyContract(doc *openapi3.T) error {
	collection := doc.Paths.Value("/api/admin/questionnaires")
	detail := doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}")
	duplicate := doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}/duplicate")
	disable := doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}/disable")
	enable := doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}/enable")
	if collection == nil || collection.Get == nil || collection.Post == nil || detail == nil || detail.Get == nil ||
		detail.Put == nil || detail.Patch == nil || detail.Delete == nil ||
		duplicate == nil || duplicate.Post == nil || disable == nil || disable.Post == nil || enable == nil || enable.Post == nil {
		return errors.New("P4-F01A Survey compatibility operations are incomplete")
	}
	if !operationResponseUsesLocalSchema(collection.Get, "LegacyQuestionnaireListResponse") ||
		!operationRequestUsesLocalSchema(collection.Post, "LegacyQuestionnaireCreateRequest") ||
		!operationResponseUsesLocalSchema(collection.Post, "LegacyQuestionnaireCreateResponse") ||
		!operationResponseUsesLocalSchema(detail.Get, "LegacyQuestionnaireDetailResponse") {
		return errors.New("P4-F01A Survey request or response schema drifted")
	}
	if err := validateRequiredCSRF(collection.Post); err != nil {
		return fmt.Errorf("createLegacyQuestionnaire: %w", err)
	}
	if collection.Post.Responses.Value("400") == nil || collection.Post.Responses.Value("409") == nil ||
		collection.Post.Responses.Value("503") == nil || detail.Get.Responses.Value("404") == nil || detail.Get.Responses.Value("503") == nil {
		return errors.New("P4-F01A Survey boundary or failure responses drifted")
	}
	for _, contract := range []struct {
		name     string
		op       *openapi3.Operation
		request  string
		response string
	}{
		{"replaceLegacyQuestionnaire", detail.Put, "LegacyQuestionnaireCreateRequest", "LegacyQuestionnaireMutationResponse"},
		{"updateLegacyQuestionnaire", detail.Patch, "LegacyQuestionnaireCreateRequest", "LegacyQuestionnaireMutationResponse"},
		{"deleteLegacyQuestionnaire", detail.Delete, "", "LegacyQuestionnaireDeleteResponse"},
		{"duplicateLegacyQuestionnaire", duplicate.Post, "LegacyQuestionnaireDuplicateRequest", "LegacyQuestionnaireMutationResponse"},
		{"disableLegacyQuestionnaire", disable.Post, "LegacyQuestionnaireDisableRequest", "LegacyQuestionnaireMutationResponse"},
		{"enableLegacyQuestionnaire", enable.Post, "", "LegacyQuestionnaireMutationResponse"},
	} {
		if contract.op.OperationID != contract.name ||
			(contract.request == "" && contract.op.RequestBody != nil) ||
			(contract.request != "" && !operationRequestUsesLocalSchema(contract.op, contract.request)) ||
			!operationResponseUsesLocalSchema(contract.op, contract.response) ||
			!hasHeader(contract.op, "Idempotency-Key") ||
			contract.op.Responses.Value("400") == nil || contract.op.Responses.Value("404") == nil ||
			contract.op.Responses.Value("409") == nil || contract.op.Responses.Value("503") == nil {
			return fmt.Errorf("%s F01B boundary, schema, or idempotency contract drifted", contract.name)
		}
		if err := validateRequiredCSRF(contract.op); err != nil {
			return fmt.Errorf("%s: %w", contract.name, err)
		}
	}
	request := doc.Components.Schemas["LegacyQuestionnaireCreateRequest"]
	if request == nil || request.Value == nil || request.Value.AdditionalProperties.Has == nil || *request.Value.AdditionalProperties.Has {
		return errors.New("LegacyQuestionnaireCreateRequest must remain closed")
	}
	assessment := request.Value.Properties["assessment_enabled"]
	config := request.Value.Properties["assessment_config"]
	rules := request.Value.Properties["score_rules"]
	if assessment == nil || assessment.Value == nil || !reflect.DeepEqual(assessment.Value.Enum, []any{false}) ||
		config == nil || config.Value == nil || config.Value.MaxProps == nil || *config.Value.MaxProps != 0 ||
		rules == nil || rules.Value == nil || rules.Value.MaxItems == nil || *rules.Value.MaxItems != 0 {
		return errors.New("P4-F01A must keep F02 assessment unavailable")
	}
	return nil
}

func validateAutomationContract(doc *openapi3.T) error {
	item := doc.Paths.Value("/api/admin/automation-conversion/agent-runs")
	if item == nil || item.Get == nil || item.Post != nil || item.Put != nil || item.Patch != nil || item.Delete != nil {
		return errors.New("P4-W0-D01 Automation compatibility operation is incomplete")
	}
	operation := item.Get
	if operation.OperationID != "listAutomationTriggerRuns" || operation.RequestBody != nil ||
		!operationResponseUsesLocalSchema(operation, "AutomationTriggerRunListResponse") ||
		operation.Responses.Value("400") == nil || operation.Responses.Value("503") == nil {
		return errors.New("P4-W0-D01 Automation response or failure contract drifted")
	}
	parameters := map[string]bool{}
	for _, parameter := range operation.Parameters {
		if parameter == nil || parameter.Value == nil || parameter.Value.In != "query" || parameters[parameter.Value.Name] {
			return errors.New("P4-W0-D01 Automation query parameters are invalid")
		}
		parameters[parameter.Value.Name] = true
	}
	for _, name := range []string{"page", "page_size", "request_id", "run_id", "agent_code", "run_status", "trigger_source", "unionid", "userid", "started_after", "started_before", "has_error", "visibility"} {
		if !parameters[name] {
			return fmt.Errorf("P4-W0-D01 Automation query is missing %s", name)
		}
	}
	if len(parameters) != 13 {
		return fmt.Errorf("P4-W0-D01 Automation query parameter count=%d", len(parameters))
	}
	for _, name := range []string{"AutomationTriggerRun", "AutomationTriggerRunListResponse"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.AdditionalProperties.Has == nil || *schema.Value.AdditionalProperties.Has {
			return fmt.Errorf("%s must remain a closed response", name)
		}
	}
	return nil
}

func validateSegmentContract(doc *openapi3.T) error {
	segments := doc.Paths.Value("/api/v1/segments")
	segment := doc.Paths.Value("/api/v1/segments/{segment_id}")
	members := doc.Paths.Value("/api/v1/segments/{segment_id}/members")
	refresh := doc.Paths.Value("/api/v1/segments/{segment_id}/refresh")
	if segments == nil || segments.Get == nil || segments.Post == nil || segment == nil || segment.Get == nil || segment.Patch == nil ||
		members == nil || members.Get == nil || refresh == nil || refresh.Post == nil {
		return errors.New("P3 segment operations are incomplete")
	}
	for _, contract := range []struct {
		operation *openapi3.Operation
		status    string
		schema    string
	}{
		{segments.Get, "200", "SegmentPage"}, {segments.Post, "201", "Segment"},
		{segment.Get, "200", "Segment"}, {segment.Patch, "200", "Segment"},
		{members.Get, "200", "SegmentMemberPage"}, {refresh.Post, "202", "SegmentRefreshAccepted"},
	} {
		if !operationResponseUsesStatusLocalSchema(contract.operation, contract.status, contract.schema) {
			return fmt.Errorf("%s response schema ref drifted", contract.operation.OperationID)
		}
	}
	for _, contract := range []struct {
		operation *openapi3.Operation
		schema    string
	}{
		{segments.Post, "CreateSegmentRequest"}, {segment.Patch, "UpdateSegmentRequest"},
	} {
		if !operationRequestUsesLocalSchema(contract.operation, contract.schema) {
			return fmt.Errorf("%s request schema ref drifted", contract.operation.OperationID)
		}
	}
	for _, operation := range []*openapi3.Operation{segments.Post, segment.Patch, refresh.Post} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", operation.OperationID, err)
		}
		if !hasRequiredHeader(operation, "Idempotency-Key") || operation.Responses.Value("409") == nil || operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s lacks idempotency, conflict, or unavailable response", operation.OperationID)
		}
	}
	for _, operation := range []*openapi3.Operation{segments.Get, segment.Get, members.Get} {
		if operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s lacks unavailable response", operation.OperationID)
		}
	}
	definition := doc.Components.Schemas["SegmentDefinition"]
	if definition == nil || definition.Value == nil || len(definition.Value.OneOf) != 3 {
		return errors.New("SegmentDefinition must remain a three-node closed AST")
	}
	for _, name := range []string{"SegmentDefinitionAnd", "SegmentDefinitionOr", "SegmentDefinitionPredicate"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.AdditionalProperties.Has == nil || *schema.Value.AdditionalProperties.Has {
			return fmt.Errorf("%s must remain a closed AST node", name)
		}
	}
	predicate := doc.Components.Schemas["SegmentDefinitionPredicate"].Value
	for field, want := range map[string][]string{
		"field": {"added_at", "channel_id", "is_deleted", "last_interact_at", "owner_staff_id", "stage_id", "tag_id"},
		"op":    {"after", "before", "eq", "has_any", "in"},
	} {
		property := predicate.Properties[field]
		if property == nil || property.Value == nil {
			return fmt.Errorf("SegmentDefinitionPredicate missing %s", field)
		}
		got, err := stringList(property.Value.Enum)
		if err != nil {
			return err
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("SegmentDefinitionPredicate.%s=%v", field, got)
		}
	}
	return nil
}

func validateIdentityContract(doc *openapi3.T) error {
	resolve := doc.Paths.Value("/api/v1/identity/resolve")
	bind := doc.Paths.Value("/api/v1/identity/bind")
	ingest := doc.Paths.Value("/api/v1/identity/ingest")
	reviews := doc.Paths.Value("/api/v1/identity/merge-reviews")
	approve := doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/approve")
	reject := doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/reject")
	if resolve == nil || resolve.Post == nil || bind == nil || bind.Post == nil || ingest == nil || ingest.Post == nil ||
		reviews == nil || reviews.Get == nil || approve == nil || approve.Post == nil || reject == nil || reject.Post == nil {
		return errors.New("P3 identity operations are incomplete")
	}
	for _, contract := range []struct {
		operation *openapi3.Operation
		request   string
		response  string
	}{
		{resolve.Post, "ResolveIdentityRequest", "ResolveIdentityResponse"},
		{bind.Post, "BindIdentityRequest", "BindIdentityResponse"},
		{ingest.Post, "IngestIdentityEventRequest", "IngestIdentityEventResponse"},
		{reviews.Get, "", "IdentityMergeReviewPage"},
		{approve.Post, "ApproveIdentityMergeReviewRequest", "IdentityMergeReview"},
		{reject.Post, "RejectIdentityMergeReviewRequest", "IdentityMergeReview"},
	} {
		if contract.request != "" && !operationRequestUsesLocalSchema(contract.operation, contract.request) {
			return fmt.Errorf("%s request schema ref drifted", contract.operation.OperationID)
		}
		if !operationResponseUsesLocalSchema(contract.operation, contract.response) {
			return fmt.Errorf("%s response schema ref drifted", contract.operation.OperationID)
		}
	}
	for name, operation := range map[string]*openapi3.Operation{
		"bindIdentity": bind.Post, "ingestIdentityEvent": ingest.Post,
		"approveIdentityMergeReview": approve.Post, "rejectIdentityMergeReview": reject.Post,
	} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if !hasRequiredHeader(operation, "Idempotency-Key") {
			return fmt.Errorf("%s lacks required Idempotency-Key", name)
		}
		if operation.Responses.Value("409") == nil || operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s lacks conflict or unavailable response", name)
		}
	}
	for _, operation := range []*openapi3.Operation{resolve.Post, reviews.Get} {
		if operation.Responses.Value("503") == nil {
			return fmt.Errorf("%s lacks unavailable response", operation.OperationID)
		}
	}
	for _, operation := range []*openapi3.Operation{resolve.Post, bind.Post, ingest.Post, approve.Post, reject.Post} {
		if operation.Responses.Value("422") == nil {
			return fmt.Errorf("%s lacks semantic validation response", operation.OperationID)
		}
	}
	ref := doc.Components.Schemas["IdentityRef"]
	if ref == nil || ref.Value == nil || ref.Value.Properties["assurance"] != nil || ref.Value.Properties["source"] != nil {
		return errors.New("admin IdentityRef must not accept assurance or source")
	}
	for _, contract := range []struct {
		name     string
		variants map[string][]string
	}{
		{"ResolveIdentityResponse", map[string][]string{
			"ResolveIdentityFound": {"customer_id", "status"}, "ResolveIdentityNotFound": {"status"},
			"ResolveIdentityConflict": {"status"},
		}},
		{"BindIdentityResponse", map[string][]string{
			"BindIdentityBound": {"customer_id", "status"}, "BindIdentityAlreadyBound": {"customer_id", "status"},
			"BindIdentityMerged":       {"customer_id", "merge_audit_id", "primary_customer_id", "status"},
			"BindIdentityManualReview": {"review_id", "status"}, "BindIdentityRejected": {"status"},
		}},
		{"IngestIdentityEventResponse", map[string][]string{
			"IngestIdentityEventAttributed": {"customer_id", "event_id", "status"},
			"IngestIdentityEventPending":    {"pending_event_id", "status"},
			"IngestIdentityEventConflict":   {"pending_event_id", "status"},
		}},
	} {
		if err := validateStrictStatusUnion(doc, contract.name, contract.variants); err != nil {
			return err
		}
	}
	review := doc.Components.Schemas["IdentityMergeReview"]
	if review == nil || review.Value == nil || review.Value.Properties["identity_fingerprint"] == nil ||
		review.Value.Properties["value"] != nil || review.Value.Properties["normalized_value"] != nil {
		return errors.New("merge review identity redaction drifted")
	}
	wantReviewFields := []string{"created_at", "customer_ids", "identity_fingerprint", "resolved_at", "review_id", "scope", "status", "type", "version"}
	gotReviewFields := make([]string, 0, len(review.Value.Properties))
	for field := range review.Value.Properties {
		gotReviewFields = append(gotReviewFields, field)
	}
	sort.Strings(gotReviewFields)
	if !reflect.DeepEqual(gotReviewFields, wantReviewFields) || review.Value.AdditionalProperties.Has == nil ||
		*review.Value.AdditionalProperties.Has {
		return fmt.Errorf("merge review response fields=%v", gotReviewFields)
	}
	reviewRequired := append([]string(nil), review.Value.Required...)
	sort.Strings(reviewRequired)
	if !reflect.DeepEqual(reviewRequired, wantReviewFields) {
		return fmt.Errorf("merge review response required=%v", reviewRequired)
	}
	customerIDs := review.Value.Properties["customer_ids"].Value
	if customerIDs == nil || customerIDs.MinItems != 2 || customerIDs.MaxItems == nil ||
		*customerIDs.MaxItems != 2 || !customerIDs.UniqueItems {
		return errors.New("merge review must contain exactly two unique current roots")
	}
	fingerprint := review.Value.Properties["identity_fingerprint"].Value
	if fingerprint == nil || fingerprint.Pattern != `^hmac-sha256-v[1-9][0-9]*:[A-Za-z0-9_-]{21}[AQgw]$` {
		return errors.New("merge review fingerprint is not a versioned secret-backed HMAC")
	}
	page := doc.Components.Schemas["IdentityMergeReviewPage"]
	if page == nil || page.Value == nil {
		return errors.New("IdentityMergeReviewPage is missing")
	}
	pageFields := make([]string, 0, len(page.Value.Properties))
	for field := range page.Value.Properties {
		pageFields = append(pageFields, field)
	}
	sort.Strings(pageFields)
	if page.Value.Type == nil || !page.Value.Type.Is("object") ||
		!reflect.DeepEqual(pageFields, []string{"items", "next_cursor"}) ||
		page.Value.AdditionalProperties.Has == nil || *page.Value.AdditionalProperties.Has {
		return fmt.Errorf("IdentityMergeReviewPage fields=%v", pageFields)
	}
	pageRequired := append([]string(nil), page.Value.Required...)
	sort.Strings(pageRequired)
	if !reflect.DeepEqual(pageRequired, []string{"items", "next_cursor"}) {
		return fmt.Errorf("IdentityMergeReviewPage required=%v", pageRequired)
	}
	items := page.Value.Properties["items"]
	nextCursor := page.Value.Properties["next_cursor"]
	if items == nil || items.Value == nil || items.Value.Type == nil || !items.Value.Type.Is("array") ||
		items.Value.Items == nil || items.Value.Items.Ref != "#/components/schemas/IdentityMergeReview" ||
		nextCursor == nil || nextCursor.Value == nil || nextCursor.Value.Type == nil ||
		!nextCursor.Value.Type.Is("string") || !nextCursor.Value.Nullable {
		return errors.New("IdentityMergeReviewPage item or cursor shape drifted")
	}
	return nil
}

func operationRequestUsesLocalSchema(operation *openapi3.Operation, schemaName string) bool {
	if operation == nil || operation.RequestBody == nil || operation.RequestBody.Value == nil {
		return false
	}
	media := operation.RequestBody.Value.Content["application/json"]
	return media != nil && media.Schema != nil && media.Schema.Ref == "#/components/schemas/"+schemaName
}

func operationResponseUsesLocalSchema(operation *openapi3.Operation, schemaName string) bool {
	return operationResponseUsesStatusLocalSchema(operation, "200", schemaName)
}

func operationResponseUsesStatusLocalSchema(operation *openapi3.Operation, status, schemaName string) bool {
	if operation == nil {
		return false
	}
	response := operation.Responses.Value(status)
	if response == nil || response.Value == nil {
		return false
	}
	media := response.Value.Content["application/json"]
	return media != nil && media.Schema != nil && media.Schema.Ref == "#/components/schemas/"+schemaName
}

func validateStrictStatusUnion(doc *openapi3.T, name string, variants map[string][]string) error {
	union := doc.Components.Schemas[name]
	if union == nil || union.Value == nil || len(union.Value.OneOf) != len(variants) ||
		union.Value.Discriminator == nil || union.Value.Discriminator.PropertyName != "status" ||
		len(union.Value.Discriminator.Mapping) != len(variants) {
		return fmt.Errorf("%s status union drifted", name)
	}
	seen := map[string]bool{}
	for _, variantRef := range union.Value.OneOf {
		const prefix = "#/components/schemas/"
		if variantRef == nil || !strings.HasPrefix(variantRef.Ref, prefix) {
			return fmt.Errorf("%s has an inline or external variant", name)
		}
		variantName := strings.TrimPrefix(variantRef.Ref, prefix)
		wantRequired, ok := variants[variantName]
		if !ok || seen[variantName] {
			return fmt.Errorf("%s has unexpected variant %s", name, variantName)
		}
		seen[variantName] = true
		variant := doc.Components.Schemas[variantName]
		if variant == nil || variant.Value == nil || variant.Value.Type == nil || !variant.Value.Type.Is("object") ||
			variant.Value.AdditionalProperties.Has == nil || *variant.Value.AdditionalProperties.Has ||
			len(variant.Value.Properties) != len(wantRequired) {
			return fmt.Errorf("%s variant %s permits ambiguous fields", name, variantName)
		}
		gotRequired := append([]string(nil), variant.Value.Required...)
		sort.Strings(gotRequired)
		if !reflect.DeepEqual(gotRequired, wantRequired) {
			return fmt.Errorf("%s variant %s required=%v", name, variantName, gotRequired)
		}
		status := variant.Value.Properties["status"]
		if status == nil || status.Value == nil || len(status.Value.Enum) != 1 {
			return fmt.Errorf("%s variant %s lacks a single status", name, variantName)
		}
		statusValue, ok := status.Value.Enum[0].(string)
		if !ok || union.Value.Discriminator.Mapping[statusValue] != prefix+variantName {
			return fmt.Errorf("%s variant %s discriminator mapping drifted", name, variantName)
		}
	}
	return nil
}

func hasRequiredHeader(operation *openapi3.Operation, name string) bool {
	for _, ref := range operation.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.In == "header" && ref.Value.Name == name && ref.Value.Required {
			return true
		}
	}
	return false
}

func hasHeader(operation *openapi3.Operation, name string) bool {
	for _, ref := range operation.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.In == "header" && ref.Value.Name == name {
			return true
		}
	}
	return false
}

func validateBrowserSessionContract(doc *openapi3.T) error {
	scheme := doc.Components.SecuritySchemes["AdminSession"]
	if scheme == nil || scheme.Value == nil || scheme.Value.Type != "apiKey" ||
		scheme.Value.In != "cookie" || scheme.Value.Name != "aicrm_session" {
		return errors.New("AdminSession must remain an opaque aicrm_session cookie")
	}
	logout := doc.Paths.Value("/api/v1/auth/logout")
	if logout == nil || logout.Post == nil {
		return errors.New("logout operation missing")
	}
	var csrf *openapi3.Parameter
	for _, ref := range logout.Post.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.Name == "X-CSRF-Token" {
			csrf = ref.Value
			break
		}
	}
	if csrf == nil || csrf.In != "header" || !csrf.Required || csrf.Schema == nil || csrf.Schema.Value == nil {
		return errors.New("logout lacks required X-CSRF-Token header")
	}
	schema := csrf.Schema.Value
	if schema.MinLength != 43 || schema.MaxLength == nil || *schema.MaxLength != 43 || schema.Pattern != "^[A-Za-z0-9_-]{43}$" {
		return errors.New("logout CSRF token shape is not frozen")
	}
	for _, status := range []string{"204", "401", "403"} {
		if logout.Post.Responses.Value(status) == nil {
			return fmt.Errorf("logout response missing: %s", status)
		}
	}
	return nil
}

func validateStageContract(doc *openapi3.T) error {
	stages := doc.Paths.Value("/api/v1/stages")
	stage := doc.Paths.Value("/api/v1/stages/{stage_id}")
	if stages == nil || stages.Get == nil || stages.Post == nil || stage == nil || stage.Patch == nil {
		return errors.New("P2 stage operations are incomplete")
	}
	for name, operation := range map[string]*openapi3.Operation{
		"createStage": stages.Post,
		"renameStage": stage.Patch,
	} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for _, status := range []string{"401", "403", "422", "503"} {
			if operation.Responses.Value(status) == nil {
				return fmt.Errorf("%s response missing: %s", name, status)
			}
		}
	}
	if stages.Post.Responses.Value("201") == nil || stage.Patch.Responses.Value("200") == nil ||
		stage.Patch.Responses.Value("404") == nil {
		return errors.New("P2 stage success or not-found responses drifted")
	}
	return nil
}

func validateContactContract(doc *openapi3.T) error {
	customers := doc.Paths.Value("/api/v1/customers")
	detail := doc.Paths.Value("/api/v1/customers/{customer_id}")
	events := doc.Paths.Value("/api/v1/customers/{customer_id}/events")
	stage := doc.Paths.Value("/api/v1/customers/{customer_id}/stage")
	tags := doc.Paths.Value("/api/v1/customers/{customer_id}/tags/{tag_id}")
	catalog := doc.Paths.Value("/api/v1/tags")
	if customers == nil || customers.Get == nil || detail == nil || detail.Get == nil || detail.Patch == nil ||
		events == nil || events.Get == nil || stage == nil || stage.Put == nil ||
		tags == nil || tags.Put == nil || tags.Delete == nil || catalog == nil || catalog.Get == nil {
		return errors.New("P3 contact operations are incomplete")
	}

	wantFilters := []string{
		"added_after", "added_before", "channel_id", "cursor", "is_deleted", "keyword", "last_interact_after",
		"last_interact_before", "limit", "owner_staff_id", "stage_id", "tag_id",
	}
	gotFilters := make([]string, 0, len(customers.Get.Parameters))
	for _, ref := range customers.Get.Parameters {
		if ref == nil || ref.Value == nil || ref.Value.In != "query" {
			return errors.New("listCustomers has invalid query parameter")
		}
		if ref.Value.Name == "offset" {
			return errors.New("listCustomers must not expose offset pagination")
		}
		gotFilters = append(gotFilters, ref.Value.Name)
	}
	sort.Strings(gotFilters)
	if fmt.Sprint(gotFilters) != fmt.Sprint(wantFilters) {
		return fmt.Errorf("listCustomers filters=%v", gotFilters)
	}

	listResponse := doc.Components.Schemas["CustomerListResponse"]
	if listResponse == nil || listResponse.Value == nil {
		return errors.New("CustomerListResponse schema missing")
	}
	required := append([]string(nil), listResponse.Value.Required...)
	sort.Strings(required)
	wantRequired := []string{"items", "next_cursor", "total", "total_is_estimate", "watermark"}
	if fmt.Sprint(required) != fmt.Sprint(wantRequired) {
		return fmt.Errorf("CustomerListResponse required=%v", required)
	}

	update := doc.Components.Schemas["CustomerUpdateRequest"]
	if update == nil || update.Value == nil {
		return errors.New("CustomerUpdateRequest schema missing")
	}
	for _, name := range []string{"stage_id", "external_userid", "unionid", "openid", "phone", "mobile"} {
		if _, ok := update.Value.Properties[name]; ok {
			return fmt.Errorf("CustomerUpdateRequest contains forbidden field: %s", name)
		}
	}

	event := doc.Components.Schemas["CustomerEvent"]
	if event == nil || event.Value == nil || event.Value.Properties["actor"] == nil {
		return errors.New("CustomerEvent actor is not frozen")
	}
	actorRequired := false
	for _, name := range event.Value.Required {
		actorRequired = actorRequired || name == "actor"
	}
	if !actorRequired {
		return errors.New("CustomerEvent actor became optional")
	}

	for name, operation := range map[string]*openapi3.Operation{
		"updateCustomer": detail.Patch, "setCustomerStage": stage.Put,
		"addCustomerTag": tags.Put, "removeCustomerTag": tags.Delete,
	} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for _, status := range []string{"401", "403"} {
			if operation.Responses.Value(status) == nil {
				return fmt.Errorf("%s response missing: %s", name, status)
			}
		}
	}
	return nil
}

func validateRequiredCSRF(operation *openapi3.Operation) error {
	var csrf *openapi3.Parameter
	for _, ref := range operation.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.Name == "X-CSRF-Token" {
			csrf = ref.Value
			break
		}
	}
	if csrf == nil || csrf.In != "header" || !csrf.Required || csrf.Schema == nil || csrf.Schema.Value == nil {
		return errors.New("required X-CSRF-Token header is missing")
	}
	schema := csrf.Schema.Value
	if schema.MinLength != 43 || schema.MaxLength == nil || *schema.MaxLength != 43 || schema.Pattern != "^[A-Za-z0-9_-]{43}$" {
		return errors.New("CSRF token shape is not frozen")
	}
	return nil
}

func stringList(value any) ([]string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result []string
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	for _, item := range result {
		if item == "" {
			return nil, errors.New("blank list item")
		}
	}
	return result, nil
}

func stringMap(value any) (map[string]string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]string
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, errors.New("empty string map")
	}
	for key, item := range result {
		if key == "" || item == "" {
			return nil, errors.New("blank string map entry")
		}
	}
	return result, nil
}
