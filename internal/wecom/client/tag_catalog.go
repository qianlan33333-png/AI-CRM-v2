package client

import (
	"context"
	"net/http"
	"net/url"
)

// TagCatalogClient reads the enterprise-contact tag directory. Construction
// is inert: it does not obtain a token or make a provider request.
type TagCatalogClient struct {
	baseURL       *url.URL
	httpClient    *http.Client
	tokenProvider TokenProvider
}

type TagCatalogClientConfig struct {
	BaseURL       string
	HTTPClient    *http.Client
	TokenProvider TokenProvider
}

// CorpTagCatalog is an observed WeCom directory payload. It is deliberately
// provider-scoped: callers must persist it only through their own snapshot
// boundary rather than treating it as the Contact-owned local tag catalog.
type CorpTagCatalog struct {
	Groups []CorpTagGroup
}

type CorpTagGroup struct {
	ProviderGroupID string
	Name            string
	Order           int32
	Tags            []CorpTag
}

type CorpTag struct {
	ProviderTagID string
	Name          string
	Order         int32
}

func NewTagCatalogClient(config TagCatalogClientConfig) (*TagCatalogClient, error) {
	baseURL, err := parseBaseURL(config.BaseURL)
	if err != nil || config.HTTPClient == nil || config.TokenProvider == nil {
		return nil, ErrInvalidConfig
	}
	return &TagCatalogClient{baseURL: baseURL, httpClient: config.HTTPClient, tokenProvider: config.TokenProvider}, nil
}

// ListCorpTags calls WeCom's read-only get_corp_tag_list endpoint. A response
// is usable only when every returned identifier and name is a safe, complete
// directory record; malformed data is not partially projected.
func (client *TagCatalogClient) ListCorpTags(ctx context.Context) (CorpTagCatalog, error) {
	if client == nil || ctx == nil {
		return CorpTagCatalog{}, ErrInvalidConfig
	}
	var payload struct {
		ErrCode   int    `json:"errcode"`
		ErrMsg    string `json:"errmsg"`
		TagGroups []struct {
			GroupID string `json:"group_id"`
			Name    string `json:"group_name"`
			Order   int32  `json:"order"`
			Tags    []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Order   int32  `json:"order"`
				Deleted bool   `json:"deleted"`
			} `json:"tag"`
		} `json:"tag_group"`
	}
	delegate := CustomerAcquisitionClient{baseURL: client.baseURL, httpClient: client.httpClient, tokenProvider: client.tokenProvider}
	if err := delegate.read(ctx, "/cgi-bin/externalcontact/get_corp_tag_list", map[string]any{}, &payload); err != nil {
		return CorpTagCatalog{}, err
	}
	if payload.TagGroups == nil || len(payload.TagGroups) > 1000 {
		return CorpTagCatalog{}, ErrUnexpectedResponse
	}

	catalog := CorpTagCatalog{Groups: make([]CorpTagGroup, 0, len(payload.TagGroups))}
	groups := make(map[string]struct{}, len(payload.TagGroups))
	tags := make(map[string]struct{})
	for _, sourceGroup := range payload.TagGroups {
		if !validCorpTagText(sourceGroup.GroupID, 128) || !validCorpTagText(sourceGroup.Name, 256) {
			return CorpTagCatalog{}, ErrUnexpectedResponse
		}
		if _, duplicate := groups[sourceGroup.GroupID]; duplicate {
			return CorpTagCatalog{}, ErrUnexpectedResponse
		}
		groups[sourceGroup.GroupID] = struct{}{}
		group := CorpTagGroup{
			ProviderGroupID: sourceGroup.GroupID,
			Name:            sourceGroup.Name,
			Order:           sourceGroup.Order,
			Tags:            make([]CorpTag, 0, len(sourceGroup.Tags)),
		}
		for _, sourceTag := range sourceGroup.Tags {
			if !validCorpTagText(sourceTag.ID, 128) || !validCorpTagText(sourceTag.Name, 256) {
				return CorpTagCatalog{}, ErrUnexpectedResponse
			}
			if _, duplicate := tags[sourceTag.ID]; duplicate {
				return CorpTagCatalog{}, ErrUnexpectedResponse
			}
			tags[sourceTag.ID] = struct{}{}
			if sourceTag.Deleted {
				continue
			}
			group.Tags = append(group.Tags, CorpTag{ProviderTagID: sourceTag.ID, Name: sourceTag.Name, Order: sourceTag.Order})
		}
		catalog.Groups = append(catalog.Groups, group)
	}
	return catalog, nil
}

func validCorpTagText(value string, limit int) bool {
	return validRequiredText(value, limit)
}
