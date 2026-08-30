package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
)

type finalEditableProjectionProof struct {
	ProductSourceCount            int64  `json:"product_source_count"`
	ProductProjectedCount         int64  `json:"product_projected_count"`
	ProductReceiptBoundCount      int64  `json:"product_receipt_bound_count"`
	ServicePeriodSourceCount      int64  `json:"service_period_source_count"`
	ServicePeriodProjectedCount   int64  `json:"service_period_projected_count"`
	ProductLegacyImageSourceCount int64  `json:"product_legacy_image_source_count"`
	ProductImageReferenceCount    int64  `json:"product_image_reference_count"`
	ProductLegacyReferenceCount   int64  `json:"product_legacy_reference_count"`
	AudienceSourceCount           int64  `json:"audience_source_count"`
	AudienceProjectedCount        int64  `json:"audience_projected_count"`
	AudienceIdentitySkippedCount  int64  `json:"audience_identity_skipped_count"`
	AudienceGroupSourceCount      int64  `json:"audience_group_source_count"`
	AudienceGroupProjectedCount   int64  `json:"audience_group_projected_count"`
	AudienceSourceMembers         int64  `json:"audience_source_members"`
	AudienceMappedMembers         int64  `json:"audience_mapped_members"`
	AudienceProjectedMembers      int64  `json:"audience_projected_members"`
	TargetDigest                  string `json:"target_digest"`
}

func verifyFinalEditableProjection(ctx context.Context, tx pgx.Tx, archiveRunID string) (finalEditableProjectionProof, error) {
	if ctx == nil || tx == nil || archiveRunID == "" {
		return finalEditableProjectionProof{}, fmt.Errorf("invalid editable projection reconciliation scope")
	}
	proof := finalEditableProjectionProof{}
	var actualProductImages int64
	err := tx.QueryRow(ctx, `
WITH source AS (
  SELECT count(*) AS product_count
  FROM public.v1_domain_import_receipts
  WHERE archive_run_id=$1 AND import_version=$2 AND table_id='public/wechat_pay_products'
    AND target_domain='product' AND target_table='products' AND disposition='import' AND verified
), projected AS (
  SELECT count(*) AS product_count,
         count(*) FILTER (WHERE receipt.target_id IS NOT NULL) AS receipt_bound,
         count(*) FILTER (WHERE projection.service_period_projected_at IS NOT NULL) AS service_period_projected,
         count(*) FILTER (WHERE projection.legacy_materials_cleared_at IS NOT NULL) AS materials_cleared,
         count(*) FILTER (WHERE
           COALESCE(item.legacy_admin_projection->'lead_program_id','null'::jsonb) <> 'null'::jsonb OR
           COALESCE(item.legacy_admin_projection->'lead_channel_id','null'::jsonb) <> 'null'::jsonb OR
           COALESCE(item.legacy_admin_projection->'completion_target','null'::jsonb) <> 'null'::jsonb OR
           COALESCE(item.legacy_admin_projection->'wecom_tagging','{}'::jsonb) <> '{}'::jsonb
         ) AS legacy_reference_count
  FROM public.product_v1_editable_projections AS projection
  JOIN public.products AS item ON item.id=projection.product_id
  LEFT JOIN public.v1_domain_import_receipts AS receipt
    ON receipt.archive_run_id=$1 AND receipt.import_version=$2
   AND receipt.table_id='public/wechat_pay_products' AND receipt.target_table='products'
   AND receipt.target_id=projection.product_id::text
   AND receipt.metadata->>'source_id'=projection.source_id::text
   AND receipt.payload_digest=projection.source_payload_digest
   AND receipt.disposition='import' AND receipt.verified
), service_period AS (
  SELECT count(*) AS source_count
  FROM public.product_service_period_history
), page_slices AS (
  SELECT count(*) FILTER (WHERE original_enabled) AS source_count
  FROM public.product_v1_page_slice_history
), product_images AS (
  SELECT count(*) AS projected_count
  FROM public.product_images AS image
  JOIN public.product_v1_editable_projections AS projection ON projection.product_id=image.product_id
)
SELECT source.product_count, projected.product_count, projected.receipt_bound,
       service_period.source_count, projected.service_period_projected,
       page_slices.source_count,
       CASE WHEN projected.materials_cleared=projected.product_count THEN product_images.projected_count ELSE -1 END,
       projected.legacy_reference_count,
       product_images.projected_count
FROM source, projected, service_period, page_slices, product_images`, archiveRunID, staticImportVersion).Scan(
		&proof.ProductSourceCount, &proof.ProductProjectedCount, &proof.ProductReceiptBoundCount,
		&proof.ServicePeriodSourceCount, &proof.ServicePeriodProjectedCount,
		&proof.ProductLegacyImageSourceCount, &proof.ProductImageReferenceCount, &proof.ProductLegacyReferenceCount, &actualProductImages)
	if err != nil {
		return finalEditableProjectionProof{}, err
	}
	if actualProductImages != proof.ProductImageReferenceCount {
		return finalEditableProjectionProof{}, fmt.Errorf("editable product material clear count mismatch")
	}
	deferredKeys := segmentapp.DeferredRedesignAudiencePackageKeys()
	var actualAudienceSourceMembers, actualAudienceMappedMembers int64
	err = tx.QueryRow(ctx, `
WITH candidate AS (
  SELECT package.id, package.group_history_id,
         (SELECT count(*) FROM public.segment_v1_audience_members AS member WHERE member.package_history_id=package.id) AS source_members,
         (SELECT count(DISTINCT member.customer_id) FROM public.segment_v1_audience_members AS member WHERE member.package_history_id=package.id AND member.customer_id IS NOT NULL) AS mapped_members
  FROM public.segment_v1_audience_packages AS package
  WHERE package.original_status='active' AND package.package_key<>ALL($1::text[])
), eligible AS (
  SELECT * FROM candidate WHERE source_members=mapped_members
), expected AS (
  SELECT count(*) AS package_count,
         count(DISTINCT group_history_id) FILTER (WHERE group_history_id IS NOT NULL) AS group_count,
         COALESCE(sum(source_members),0)::bigint AS source_members,
         COALESCE(sum(mapped_members),0)::bigint AS mapped_members
  FROM eligible
), actual AS (
  SELECT count(*) AS package_count,
         COALESCE(sum(projection.source_member_count),0)::bigint AS source_members,
         COALESCE(sum(projection.mapped_member_count),0)::bigint AS mapped_members,
         COALESCE(sum((SELECT count(*) FROM public.segment_members AS member WHERE member.segment_id=projection.segment_id)),0)::bigint AS current_members
  FROM public.ai_audience_v1_editable_package_projections AS projection
)
SELECT (SELECT count(*) FROM candidate), actual.package_count,
       (SELECT count(*) FROM candidate WHERE source_members<>mapped_members),
       expected.group_count, (SELECT count(*) FROM public.ai_audience_v1_editable_group_projections),
       expected.source_members, expected.mapped_members, actual.current_members,
       actual.source_members, actual.mapped_members
FROM expected, actual`, deferredKeys).Scan(
		&proof.AudienceSourceCount, &proof.AudienceProjectedCount,
		&proof.AudienceIdentitySkippedCount,
		&proof.AudienceGroupSourceCount, &proof.AudienceGroupProjectedCount,
		&proof.AudienceSourceMembers, &proof.AudienceMappedMembers, &proof.AudienceProjectedMembers,
		&actualAudienceSourceMembers, &actualAudienceMappedMembers)
	if err != nil {
		return finalEditableProjectionProof{}, err
	}
	if actualAudienceSourceMembers != proof.AudienceSourceMembers || actualAudienceMappedMembers != proof.AudienceMappedMembers {
		return finalEditableProjectionProof{}, fmt.Errorf("editable Audience projection source counts mismatch")
	}
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	productRows, err := tx.Query(ctx, `
SELECT projection.source_id,encode(projection.source_payload_digest,'hex'),projection.product_id,
       item.product_code,item.name,item.description,item.price_minor,item.currency,item.stock_quantity,
       item.local_lifecycle,item.version,item.legacy_admin_projection::text,
       COALESCE(projection.service_period_definition_id,0),projection.service_period_projected_at IS NOT NULL,
       projection.legacy_materials_cleared_at IS NOT NULL,projection.cleared_material_reference_count,
       COALESCE((SELECT jsonb_agg(jsonb_build_array(image.position,image.image_url) ORDER BY image.position)::text
                 FROM public.product_images AS image WHERE image.product_id=projection.product_id),'[]')
FROM public.product_v1_editable_projections AS projection
JOIN public.products AS item ON item.id=projection.product_id
ORDER BY projection.source_id`)
	if err != nil {
		return finalEditableProjectionProof{}, err
	}
	defer productRows.Close()
	for productRows.Next() {
		var sourceID, productID, price, stock, version, serviceDefinition, imageCount int64
		var sourceDigest, code, name, description, currency, lifecycle, admin, images string
		var serviceProjected, imagesProjected bool
		if err = productRows.Scan(&sourceID, &sourceDigest, &productID, &code, &name, &description, &price, &currency, &stock, &lifecycle, &version, &admin, &serviceDefinition, &serviceProjected, &imagesProjected, &imageCount, &images); err != nil {
			return finalEditableProjectionProof{}, err
		}
		if err = encoder.Encode([]any{"product", sourceID, sourceDigest, productID, code, name, description, price, currency, stock, lifecycle, version, admin, serviceDefinition, serviceProjected, imagesProjected, imageCount, images}); err != nil {
			return finalEditableProjectionProof{}, err
		}
	}
	if err = productRows.Err(); err != nil {
		return finalEditableProjectionProof{}, err
	}
	audienceRows, err := tx.Query(ctx, `
SELECT package.source_id,projection.segment_id,projection.source_member_count,projection.mapped_member_count,
       segment.name,segment.definition::text,segment.member_count,metadata.lifecycle,metadata.version,
       COALESCE(metadata.group_id,0),
       COALESCE((SELECT jsonb_agg(member.customer_id ORDER BY member.customer_id)::text FROM public.segment_members AS member WHERE member.segment_id=projection.segment_id),'[]')
FROM public.ai_audience_v1_editable_package_projections AS projection
JOIN public.segment_v1_audience_packages AS package ON package.id=projection.package_history_id
JOIN public.segments AS segment ON segment.id=projection.segment_id
JOIN public.ai_audience_package_metadata AS metadata ON metadata.segment_id=projection.segment_id
ORDER BY package.source_id`)
	if err != nil {
		return finalEditableProjectionProof{}, err
	}
	defer audienceRows.Close()
	for audienceRows.Next() {
		var sourceID, segmentID, sourceMembers, mappedMembers, memberCount, version, groupID int64
		var name, definition, lifecycle, members string
		if err = audienceRows.Scan(&sourceID, &segmentID, &sourceMembers, &mappedMembers, &name, &definition, &memberCount, &lifecycle, &version, &groupID, &members); err != nil {
			return finalEditableProjectionProof{}, err
		}
		if lifecycle != "paused" || memberCount != mappedMembers {
			return finalEditableProjectionProof{}, fmt.Errorf("editable audience projection is not paused or count-bound")
		}
		if err = encoder.Encode([]any{"audience", sourceID, segmentID, sourceMembers, mappedMembers, name, definition, memberCount, lifecycle, version, groupID, members}); err != nil {
			return finalEditableProjectionProof{}, err
		}
	}
	if err = audienceRows.Err(); err != nil {
		return finalEditableProjectionProof{}, err
	}
	proof.TargetDigest = hex.EncodeToString(hash.Sum(nil))
	if err = validateFinalEditableProjectionProof(proof); err != nil {
		return finalEditableProjectionProof{}, err
	}
	return proof, nil
}

func validateFinalEditableProjectionProof(proof finalEditableProjectionProof) error {
	digest, err := hex.DecodeString(proof.TargetDigest)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("editable projection target digest is invalid")
	}
	if proof.ProductSourceCount < 1 || proof.ProductProjectedCount != proof.ProductSourceCount || proof.ProductReceiptBoundCount != proof.ProductSourceCount {
		return fmt.Errorf("editable Product definitions are not receipt-bound and complete")
	}
	if proof.ServicePeriodProjectedCount != proof.ServicePeriodSourceCount {
		return fmt.Errorf("editable service-period definitions are incomplete")
	}
	if proof.ProductImageReferenceCount != 0 || proof.ProductLegacyReferenceCount != 0 {
		return fmt.Errorf("editable Product retains legacy material references")
	}
	if proof.AudienceSourceCount != proof.AudienceProjectedCount+proof.AudienceIdentitySkippedCount || proof.AudienceGroupProjectedCount != proof.AudienceGroupSourceCount ||
		proof.AudienceSourceMembers != proof.AudienceMappedMembers || proof.AudienceMappedMembers != proof.AudienceProjectedMembers {
		return fmt.Errorf("editable Audience definitions are incomplete")
	}
	return nil
}
