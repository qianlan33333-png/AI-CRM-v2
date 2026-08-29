package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"

	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

type CustomerAcquisitionLinkProvider struct{ client *CustomerAcquisitionClient }

var _ wecomport.CustomerAcquisitionLinkProvider = (*CustomerAcquisitionLinkProvider)(nil)

func NewCustomerAcquisitionLinkProvider(client *CustomerAcquisitionClient) (*CustomerAcquisitionLinkProvider, error) {
	if client == nil {
		return nil, ErrInvalidConfig
	}
	return &CustomerAcquisitionLinkProvider{client: client}, nil
}

func (provider *CustomerAcquisitionLinkProvider) ListCustomerAcquisitionLinks(ctx context.Context, cursor string, limit int) (wecomport.CustomerAcquisitionLinkPage, error) {
	if provider == nil || provider.client == nil {
		return wecomport.CustomerAcquisitionLinkPage{}, ErrInvalidConfig
	}
	page, err := provider.client.ListCustomerAcquisitionLinks(ctx, cursor, limit)
	if err != nil {
		return wecomport.CustomerAcquisitionLinkPage{}, err
	}
	result := wecomport.CustomerAcquisitionLinkPage{Links: make([]wecomport.CustomerAcquisitionLink, len(page.Links)), NextCursor: page.NextCursor}
	for index, link := range page.Links {
		result.Links[index] = customerAcquisitionLinkPortValue(link)
	}
	return result, nil
}

func (provider *CustomerAcquisitionLinkProvider) GetCustomerAcquisitionLink(ctx context.Context, linkID string) (wecomport.CustomerAcquisitionLink, error) {
	if provider == nil || provider.client == nil {
		return wecomport.CustomerAcquisitionLink{}, ErrInvalidConfig
	}
	link, err := provider.client.GetCustomerAcquisitionLink(ctx, linkID)
	if err != nil {
		return wecomport.CustomerAcquisitionLink{}, err
	}
	return customerAcquisitionLinkPortValue(link), nil
}

func (provider *CustomerAcquisitionLinkProvider) CreateCustomerAcquisitionLink(ctx context.Context, input wecomport.CustomerAcquisitionLinkInput) (wecomport.CustomerAcquisitionLinkWriteResult, error) {
	if provider == nil || provider.client == nil {
		return wecomport.CustomerAcquisitionLinkWriteResult{}, ErrInvalidConfig
	}
	request := customerAcquisitionLinkClientInput(input)
	link, err := provider.client.CreateCustomerAcquisitionLink(ctx, request)
	return customerAcquisitionLinkWriteResult("create", "", request, link, err)
}

func (provider *CustomerAcquisitionLinkProvider) UpdateCustomerAcquisitionLink(ctx context.Context, linkID string, input wecomport.CustomerAcquisitionLinkInput) (wecomport.CustomerAcquisitionLinkWriteResult, error) {
	if provider == nil || provider.client == nil {
		return wecomport.CustomerAcquisitionLinkWriteResult{}, ErrInvalidConfig
	}
	request := customerAcquisitionLinkClientInput(input)
	link, err := provider.client.UpdateCustomerAcquisitionLink(ctx, linkID, request)
	return customerAcquisitionLinkWriteResult("update", linkID, request, link, err)
}

func (provider *CustomerAcquisitionLinkProvider) DeleteCustomerAcquisitionLink(ctx context.Context, linkID string) (wecomport.CustomerAcquisitionLinkWriteResult, error) {
	if provider == nil || provider.client == nil {
		return wecomport.CustomerAcquisitionLinkWriteResult{}, ErrInvalidConfig
	}
	err := provider.client.DeleteCustomerAcquisitionLink(ctx, linkID)
	return customerAcquisitionLinkWriteResult("delete", linkID, CustomerAcquisitionLinkRequest{}, CustomerAcquisitionLink{}, err)
}

func customerAcquisitionLinkWriteResult(operation, linkID string, input CustomerAcquisitionLinkRequest, link CustomerAcquisitionLink, err error) (wecomport.CustomerAcquisitionLinkWriteResult, error) {
	requestDigest := customerAcquisitionLinkProviderDigest(operation, linkID, input)
	switch {
	case err == nil:
		result := wecomport.CustomerAcquisitionLinkWriteResult{
			Outcome: wecomport.CustomerAcquisitionLinkExecuted, ReceiptDigest: customerAcquisitionLinkReceiptDigest("executed", requestDigest, 0),
			BusinessEndpointDispatched: true, RealExternalCallExecuted: true,
		}
		if operation != "delete" {
			value := customerAcquisitionLinkPortValue(link)
			result.Link = &value
		}
		return result, nil
	case errors.Is(err, ErrWriteOutcomeUnknown):
		return wecomport.CustomerAcquisitionLinkWriteResult{
			Outcome: wecomport.CustomerAcquisitionLinkOutcomeUnknown, ReceiptDigest: customerAcquisitionLinkReceiptDigest("outcome_unknown", requestDigest, 0),
			BusinessEndpointDispatched: true,
		}, nil
	case errors.Is(err, ErrUpstream):
		var providerError *APIError
		if !errors.As(err, &providerError) || providerError.Code == 0 {
			return wecomport.CustomerAcquisitionLinkWriteResult{}, err
		}
		return wecomport.CustomerAcquisitionLinkWriteResult{
			Outcome: wecomport.CustomerAcquisitionLinkFinalFailed, ReceiptDigest: customerAcquisitionLinkReceiptDigest("final_failed", requestDigest, providerError.Code),
			BusinessEndpointDispatched: true, RealExternalCallExecuted: true,
		}, nil
	case errors.Is(err, ErrBusinessWriteNotDispatched):
		return wecomport.CustomerAcquisitionLinkWriteResult{}, wecomport.ErrCustomerAcquisitionLinkNotDispatched
	default:
		return wecomport.CustomerAcquisitionLinkWriteResult{}, err
	}
}

func customerAcquisitionLinkClientInput(input wecomport.CustomerAcquisitionLinkInput) CustomerAcquisitionLinkRequest {
	return CustomerAcquisitionLinkRequest{LinkName: input.LinkName, UserIDs: append([]string(nil), input.UserIDs...), DepartmentIDs: append([]int64(nil), input.DepartmentIDs...), SkipVerify: input.SkipVerify}
}

func customerAcquisitionLinkPortValue(link CustomerAcquisitionLink) wecomport.CustomerAcquisitionLink {
	return wecomport.CustomerAcquisitionLink{LinkID: link.LinkID, LinkName: link.LinkName, URL: link.URL, UserIDs: append([]string(nil), link.UserIDs...), DepartmentIDs: append([]int64(nil), link.DepartmentIDs...), SkipVerify: link.SkipVerify}
}

func customerAcquisitionLinkProviderDigest(operation, linkID string, input CustomerAcquisitionLinkRequest) [32]byte {
	return sha256.Sum256([]byte("wecom.customer_acquisition.link.provider.v1\x00" + operation + "\x00" + linkID + "\x00" + input.LinkName + "\x00" + stringsJoin(input.UserIDs) + "\x00" + int64sJoin(input.DepartmentIDs) + "\x00" + strconv.FormatBool(input.SkipVerify)))
}

func customerAcquisitionLinkReceiptDigest(state string, request [32]byte, providerCode int) [32]byte {
	return sha256.Sum256([]byte("wecom.customer_acquisition.link.receipt.v1\x00" + state + "\x00" + hex.EncodeToString(request[:]) + "\x00" + strconv.Itoa(providerCode)))
}

func stringsJoin(values []string) string {
	encoded := ""
	for _, value := range values {
		encoded += strconv.Itoa(len(value)) + ":" + value
	}
	return encoded
}

func int64sJoin(values []int64) string {
	encoded := ""
	for _, value := range values {
		encoded += strconv.FormatInt(value, 10) + ","
	}
	return encoded
}
