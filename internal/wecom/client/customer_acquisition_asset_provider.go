package client

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

// CustomerAcquisitionAssetProvider is the only CH02 translation from a
// Contact-owned immutable snapshot to WeCom's customer-acquisition writes.
// It returns digests only; provider IDs and URLs never cross the port.
type CustomerAcquisitionAssetProvider struct {
	client *CustomerAcquisitionClient
}

var _ contactport.AcquisitionAssetProvider = (*CustomerAcquisitionAssetProvider)(nil)

func NewCustomerAcquisitionAssetProvider(client *CustomerAcquisitionClient) (*CustomerAcquisitionAssetProvider, error) {
	if client == nil {
		return nil, ErrInvalidConfig
	}
	return &CustomerAcquisitionAssetProvider{client: client}, nil
}

func (provider *CustomerAcquisitionAssetProvider) PublishAcquisitionAsset(ctx context.Context, request contactport.AcquisitionAssetPublishRequest) (contactport.AcquisitionAssetProviderResult, error) {
	if provider == nil || provider.client == nil || ctx == nil || !validAcquisitionAssetPublishRequest(request) {
		return contactport.AcquisitionAssetProviderResult{}, ErrInvalidConfig
	}
	requestDigest := acquisitionAssetRequestDigest(request)
	switch request.Kind {
	case contactport.AcquisitionAssetQRCode:
		published, err := provider.client.PublishContactWay(ctx, ContactWayRequest{
			Type: 2, Scene: 2, Remark: request.Snapshot.ChannelName, State: request.CorrelationKey,
			UserIDs: append([]string(nil), request.Snapshot.AssigneeWeComUserIDs...),
		})
		if err != nil {
			return acquisitionAssetProviderFailure(requestDigest, err)
		}
		return acquisitionAssetProviderSuccess(requestDigest, published.ConfigID, published.QRCodeURL), nil
	case contactport.AcquisitionAssetLink:
		published, err := provider.client.CreateCustomerAcquisitionLink(ctx, CustomerAcquisitionLinkRequest{
			LinkName: request.Snapshot.ChannelName, UserIDs: append([]string(nil), request.Snapshot.AssigneeWeComUserIDs...),
		})
		if err != nil {
			return acquisitionAssetProviderFailure(requestDigest, err)
		}
		finalURL, err := appendCustomerChannel(published.URL, request.CorrelationKey)
		if err != nil {
			return contactport.AcquisitionAssetProviderResult{BusinessEndpointDispatched: true, RealExternalCallExecuted: true}, err
		}
		return acquisitionAssetProviderSuccess(requestDigest, published.LinkID, finalURL), nil
	default:
		return contactport.AcquisitionAssetProviderResult{}, ErrInvalidConfig
	}
}

func acquisitionAssetProviderSuccess(requestDigest [32]byte, providerID, providerURL string) contactport.AcquisitionAssetProviderResult {
	reference := sha256.Sum256([]byte("wecom.customer_acquisition.asset.reference.v1\x00" + providerID))
	receipt := sha256.Sum256([]byte("wecom.customer_acquisition.asset.receipt.v1\x00executed\x00" + hex.EncodeToString(requestDigest[:]) + "\x00" + hex.EncodeToString(reference[:]) + "\x00" + acquisitionAssetTextDigest(providerURL)))
	return contactport.AcquisitionAssetProviderResult{
		Outcome: contactport.AcquisitionAssetProviderExecuted, ReceiptDigest: receipt,
		AssetReferenceDigest: reference, BusinessEndpointDispatched: true, RealExternalCallExecuted: true,
	}
}

func acquisitionAssetProviderFailure(requestDigest [32]byte, err error) (contactport.AcquisitionAssetProviderResult, error) {
	switch {
	case errors.Is(err, ErrBusinessWriteNotDispatched):
		return contactport.AcquisitionAssetProviderResult{}, err
	case errors.Is(err, ErrWriteOutcomeUnknown):
		receipt := sha256.Sum256([]byte("wecom.customer_acquisition.asset.receipt.v1\x00outcome_unknown\x00" + hex.EncodeToString(requestDigest[:])))
		return contactport.AcquisitionAssetProviderResult{Outcome: contactport.AcquisitionAssetProviderOutcomeUnknown, ReceiptDigest: receipt, BusinessEndpointDispatched: true}, nil
	case errors.Is(err, ErrUpstream):
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Code == 0 {
			return contactport.AcquisitionAssetProviderResult{}, err
		}
		receipt := sha256.Sum256([]byte("wecom.customer_acquisition.asset.receipt.v1\x00final_failed\x00" + hex.EncodeToString(requestDigest[:]) + "\x00" + strconv.Itoa(apiErr.Code)))
		return contactport.AcquisitionAssetProviderResult{
			Outcome: contactport.AcquisitionAssetProviderFinalFailed, ReceiptDigest: receipt, BusinessEndpointDispatched: true, RealExternalCallExecuted: true,
		}, nil
	default:
		return contactport.AcquisitionAssetProviderResult{}, err
	}
}

func validAcquisitionAssetPublishRequest(request contactport.AcquisitionAssetPublishRequest) bool {
	if request.EffectID == "" || !validRequiredText(request.CorpID, 128) || !validCorrelationKey(request.CorrelationKey) || request.AssetVersion < 1 || request.Supersedes < 0 || request.AssetVersion <= request.Supersedes ||
		request.Snapshot.ChannelID < 1 || request.Snapshot.ChannelRevision < 1 || request.Snapshot.ChannelStatus != "active" ||
		request.SnapshotDigest == ([32]byte{}) || !validRequiredText(request.Snapshot.ChannelCode, 200) ||
		!validRequiredText(request.Snapshot.ChannelName, 120) || !validStringSlice(request.Snapshot.AssigneeWeComUserIDs, 1, 200) {
		return false
	}
	switch request.Kind {
	case contactport.AcquisitionAssetQRCode:
		return validRequiredText(request.Snapshot.SceneValue, 120)
	case contactport.AcquisitionAssetLink:
		return validOptionalText(request.Snapshot.SceneValue, 512)
	default:
		return false
	}
}

func acquisitionAssetRequestDigest(request contactport.AcquisitionAssetPublishRequest) [32]byte {
	assignees := append([]string(nil), request.Snapshot.AssigneeWeComUserIDs...)
	sort.Strings(assignees)
	canonical := strings.Join([]string{
		request.EffectID, request.CorpID, request.CorrelationKey, strconv.FormatInt(request.AssetVersion, 10), strconv.FormatInt(request.Supersedes, 10), string(request.Kind),
		strconv.FormatInt(request.Snapshot.ChannelID, 10), strconv.FormatInt(request.Snapshot.ChannelRevision, 10), request.Snapshot.ChannelCode,
		request.Snapshot.ChannelName, request.Snapshot.ChannelStatus, request.Snapshot.SceneValue, strings.Join(assignees, "\x00"), hex.EncodeToString(request.SnapshotDigest[:]),
	}, "\x00")
	return sha256.Sum256([]byte("wecom.customer_acquisition.asset.request.v1\x00" + canonical))
}

func validCorrelationKey(value string) bool {
	if !strings.HasPrefix(value, "ch02_") || len(value) != len("ch02_")+43 || !validRequiredText(value, 48) {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "ch02_"))
	return err == nil && len(raw) == 32
}

func appendCustomerChannel(value, correlationKey string) (string, error) {
	if !validOpaqueHTTPSURL(value) || !validCorrelationKey(correlationKey) {
		return "", ErrWriteOutcomeUnknown
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", ErrWriteOutcomeUnknown
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", ErrWriteOutcomeUnknown
	}
	if query.Has("customer_channel") {
		return "", ErrWriteOutcomeUnknown
	}
	query.Set("customer_channel", correlationKey)
	parsed.RawQuery = query.Encode()
	final := parsed.String()
	if !validOpaqueHTTPSURL(final) {
		return "", ErrWriteOutcomeUnknown
	}
	return final, nil
}

func acquisitionAssetTextDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
