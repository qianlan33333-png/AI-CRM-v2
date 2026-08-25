package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCustomerAcquisitionClientUsesFrozenWeComContracts(t *testing.T) {
	provider := staticTokenProvider{token: AccessToken{value: "token-safe"}}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Query().Get("access_token") != "token-safe" || request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("request=%s query=%q content-type=%q", request.Method, request.URL.RawQuery, request.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		switch request.URL.Path {
		case "/cgi-bin/externalcontact/add_contact_way":
			want := `{"party":[12],"remark":"Summer","scene":2,"skip_verify":true,"state":"channel-a","type":2,"user":["staff-a"]}`
			if string(body) != want {
				t.Fatalf("add body=%s want=%s", body, want)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"config_id":"config-1","qr_code":"https://work.weixin.qq.com/q/config-1"}`))
		case "/cgi-bin/externalcontact/get_contact_way":
			if string(body) != `{"config_id":"config-1"}` {
				t.Fatalf("get contact way body=%s", body)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"contact_way":{"type":2,"scene":2,"qr_code":"https://work.weixin.qq.com/q/config-1","skip_verify":true,"state":"channel-a","user":["staff-a"],"party":[12]}}`))
		case "/cgi-bin/externalcontact/list_contact_way":
			if string(body) != `{"cursor":"prior","limit":2}` {
				t.Fatalf("list contact ways body=%s", body)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"contact_way":[{"config_id":"config-1"},{"config_id":"config-2"}],"next_cursor":"next"}`))
		case "/cgi-bin/externalcontact/customer_acquisition/create_link":
			want := `{"link_name":"CH02 landing","range":{"department_list":[12],"user_list":["staff-a"]},"skip_verify":true}`
			if string(body) != want {
				t.Fatalf("create link body=%s want=%s", body, want)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"link_id":"link-1","url":"https://work.weixin.qq.com/ca/link-1"}`))
		case "/cgi-bin/externalcontact/customer_acquisition/get":
			if string(body) != `{"link_id":"link-1"}` {
				t.Fatalf("get link body=%s", body)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"link":{"link_id":"link-1","link_name":"CH02 landing","url":"https://work.weixin.qq.com/ca/link-1","skip_verify":true,"range":{"user_list":["staff-a"],"department_list":[12]}}}`))
		case "/cgi-bin/externalcontact/customer_acquisition/list_link":
			if string(body) != `{"cursor":"prior","limit":2}` {
				t.Fatalf("list links body=%s", body)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"link":[{"link_id":"link-1"},{"link_id":"link-2"}],"next_cursor":"next"}`))
		default:
			t.Fatalf("unexpected path=%s", request.URL.Path)
		}
	}))
	defer server.Close()
	client := testCustomerAcquisitionClient(t, server.URL, server.Client(), provider)

	contact, err := client.PublishContactWay(context.Background(), ContactWayRequest{Type: 2, Scene: 2, Remark: "Summer", SkipVerify: true, State: "channel-a", UserIDs: []string{"staff-a"}, PartyIDs: []int64{12}})
	if err != nil || contact.ConfigID != "config-1" || contact.QRCodeURL != "https://work.weixin.qq.com/q/config-1" {
		t.Fatalf("PublishContactWay()=%+v err=%v", contact, err)
	}
	if contact, err = client.ReconcileContactWay(context.Background(), "config-1"); err != nil || contact.Type != 2 || len(contact.UserIDs) != 1 {
		t.Fatalf("ReconcileContactWay()=%+v err=%v", contact, err)
	}
	if page, err := client.ListContactWays(context.Background(), "prior", 2); err != nil || page.NextCursor != "next" || len(page.ContactWays) != 2 {
		t.Fatalf("ListContactWays()=%+v err=%v", page, err)
	}

	link, err := client.CreateCustomerAcquisitionLink(context.Background(), CustomerAcquisitionLinkRequest{LinkName: "CH02 landing", UserIDs: []string{"staff-a"}, DepartmentIDs: []int64{12}, SkipVerify: true})
	if err != nil || link.LinkID != "link-1" || link.URL != "https://work.weixin.qq.com/ca/link-1" {
		t.Fatalf("CreateCustomerAcquisitionLink()=%+v err=%v", link, err)
	}
	if link, err = client.ReconcileCustomerAcquisitionLink(context.Background(), "link-1"); err != nil || link.LinkName != "CH02 landing" || len(link.DepartmentIDs) != 1 {
		t.Fatalf("ReconcileCustomerAcquisitionLink()=%+v err=%v", link, err)
	}
	if page, err := client.ListCustomerAcquisitionLinks(context.Background(), "prior", 2); err != nil || page.NextCursor != "next" || len(page.Links) != 2 {
		t.Fatalf("ListCustomerAcquisitionLinks()=%+v err=%v", page, err)
	}
}

func TestCustomerAcquisitionWritesBecomeUnknownAfterDispatch(t *testing.T) {
	for _, name := range []string{"timeout", "disconnect", "invalid-json", "trailing-json", "non-200", "oversized"} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				switch name {
				case "timeout":
					// The client deadline fires before this response. A bounded wait
					// avoids leaving httptest.Server with an active handler on Close.
					time.Sleep(50 * time.Millisecond)
				case "disconnect":
					hijacker := writer.(http.Hijacker)
					connection, _, err := hijacker.Hijack()
					if err != nil {
						t.Fatal(err)
					}
					_ = connection.Close()
				case "invalid-json":
					_, _ = writer.Write([]byte(`not-json`))
				case "trailing-json":
					_, _ = writer.Write([]byte(`{"errcode":0,"link_id":"link-1","url":"https://work.weixin.qq.com/ca/link-1"} {}`))
				case "non-200":
					writer.WriteHeader(http.StatusBadGateway)
				case "oversized":
					_, _ = writer.Write([]byte(`{"errcode":0,"link_id":"link-1","url":"https://work.weixin.qq.com/ca/link-1","padding":"` + strings.Repeat("x", maxResponseBytes) + `"}`))
				}
			}))
			defer server.Close()
			client := testCustomerAcquisitionClient(t, server.URL, server.Client(), staticTokenProvider{token: AccessToken{value: "token-safe"}})
			ctx := context.Background()
			if name == "timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
			}
			_, err := client.CreateCustomerAcquisitionLink(ctx, CustomerAcquisitionLinkRequest{LinkName: "known", UserIDs: []string{"staff-a"}})
			if !errors.Is(err, ErrWriteOutcomeUnknown) {
				t.Fatalf("CreateCustomerAcquisitionLink() err=%v, want unknown", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("write calls=%d, want no retry after unknown outcome", calls.Load())
			}
		})
	}
}

func TestCustomerAcquisitionReadTokenExpiryRefreshesAtMostOnce(t *testing.T) {
	var calls atomic.Int32
	provider := refreshTokenProvider{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if got := request.URL.Query().Get("access_token"); got == "expired" {
			_, _ = writer.Write([]byte(`{"errcode":42001,"errmsg":"expired"}`))
			return
		}
		if got := request.URL.Query().Get("access_token"); got != "fresh" {
			t.Fatalf("access_token=%q", got)
		}
		_, _ = writer.Write([]byte(`{"errcode":0,"link":{"link_id":"link-1","link_name":"known","url":"https://work.weixin.qq.com/ca/link-1","range":{"user_list":["staff-a"],"department_list":[]}}}`))
	}))
	defer server.Close()
	client := testCustomerAcquisitionClient(t, server.URL, server.Client(), &provider)
	if _, err := client.GetCustomerAcquisitionLink(context.Background(), "link-1"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || provider.refreshes.Load() != 1 {
		t.Fatalf("calls=%d refreshes=%d", calls.Load(), provider.refreshes.Load())
	}
}

func TestCustomerAcquisitionWriteTokenExpiryIsUnknownWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	provider := refreshTokenProvider{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		_, _ = writer.Write([]byte(`{"errcode":42001,"errmsg":"expired"}`))
	}))
	defer server.Close()
	client := testCustomerAcquisitionClient(t, server.URL, server.Client(), &provider)
	_, err := client.CreateCustomerAcquisitionLink(context.Background(), CustomerAcquisitionLinkRequest{LinkName: "known", UserIDs: []string{"staff-a"}})
	if !errors.Is(err, ErrWriteOutcomeUnknown) || calls.Load() != 1 || provider.refreshes.Load() != 0 {
		t.Fatalf("err=%v calls=%d refreshes=%d", err, calls.Load(), provider.refreshes.Load())
	}
}

func TestCustomerAcquisitionReadsFailClosedAndDoNotLeakToken(t *testing.T) {
	secret := "token-must-not-leak"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"errcode":0,"link":{"link_id":"link-1","link_name":"bad","url":"http://unsafe.example.test","range":{"user_list":[]}}}`))
	}))
	defer server.Close()
	client := testCustomerAcquisitionClient(t, server.URL, server.Client(), staticTokenProvider{token: AccessToken{value: secret}})
	_, err := client.GetCustomerAcquisitionLink(context.Background(), "link-1")
	if !errors.Is(err, ErrUnexpectedResponse) || strings.Contains(err.Error(), secret) {
		t.Fatalf("GetCustomerAcquisitionLink() err=%v", err)
	}
	if _, err := client.ListCustomerAcquisitionLinks(context.Background(), "bad\n", 2); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid cursor err=%v", err)
	}
	if _, err := client.PublishContactWay(context.Background(), ContactWayRequest{Type: 1, Scene: 2, UserIDs: []string{"staff-a", "staff-b"}}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid contact way err=%v", err)
	}
	if _, err := client.CreateCustomerAcquisitionLink(context.Background(), CustomerAcquisitionLinkRequest{LinkName: "link", UserIDs: []string{"staff-a", "staff-a"}}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid acquisition range err=%v", err)
	}
	if !validContactWayRequest(ContactWayRequest{Type: 2, Scene: 2, PartyIDs: []int64{12}}) {
		t.Fatal("Type=2 department-only contact way was rejected")
	}
	if validContactWayRequest(ContactWayRequest{Type: 1, Scene: 2, UserIDs: []string{"staff-a"}, PartyIDs: []int64{12}}) {
		t.Fatal("Type=1 contact way accepted party assignment")
	}
}

func TestCustomerAcquisitionReadTransportErrorRedactsAccessToken(t *testing.T) {
	secret := "token-must-not-leak"
	client := testCustomerAcquisitionClient(t, "https://qyapi.weixin.qq.com", &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Post", URL: "https://redacted.invalid/request?access_token=" + secret, Err: io.ErrUnexpectedEOF}
	})}, staticTokenProvider{token: AccessToken{value: secret}})
	_, err := client.GetCustomerAcquisitionLink(context.Background(), "link-1")
	if !errors.Is(err, ErrTransport) || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "access_token") {
		t.Fatalf("GetCustomerAcquisitionLink() err=%v", err)
	}
}

func TestDisabledCustomerAcquisitionClientNeverGetsTokenOrCallsHTTP(t *testing.T) {
	disabled := NewDisabledCustomerAcquisitionClient()
	if _, err := disabled.PublishContactWay(context.Background(), ContactWayRequest{}); !errors.Is(err, ErrCustomerAcquisitionDisabled) {
		t.Fatalf("PublishContactWay() err=%v", err)
	}
	if _, err := disabled.CreateCustomerAcquisitionLink(context.Background(), CustomerAcquisitionLinkRequest{}); !errors.Is(err, ErrCustomerAcquisitionDisabled) {
		t.Fatalf("CreateCustomerAcquisitionLink() err=%v", err)
	}
}

func testCustomerAcquisitionClient(t *testing.T, baseURL string, httpClient *http.Client, provider TokenProvider) *CustomerAcquisitionClient {
	t.Helper()
	client, err := NewCustomerAcquisitionClient(CustomerAcquisitionClientConfig{BaseURL: baseURL, HTTPClient: httpClient, TokenProvider: provider})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

type refreshTokenProvider struct{ refreshes atomic.Int32 }

func (*refreshTokenProvider) Token(context.Context) (AccessToken, error) {
	return AccessToken{value: "expired"}, nil
}
func (provider *refreshTokenProvider) RefreshToken(context.Context) (AccessToken, error) {
	provider.refreshes.Add(1)
	return AccessToken{value: "fresh"}, nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
