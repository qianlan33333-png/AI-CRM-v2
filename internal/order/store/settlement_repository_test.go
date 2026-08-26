package store

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderdb "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store/generated"
)

func TestMapPaymentMapsDatabasePrepayReadyToExecutedHandoff(t *testing.T) {
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	row := orderdb.OrderPaymentCommand{
		ID: 1, OrderID: 2, State: "prepay_ready", ProviderPrepayDigest: make([]byte, 32), Version: 3,
		CreatedAt: pgTime(now), UpdatedAt: pgTime(now), ProviderJsapiContractVersion: textValue("wechat-jsapi/v1"),
		ProviderJsapiAppID: textValue("wx-app"), ProviderJsapiTimestamp: int8Value(now.Unix()), ProviderJsapiNonceStr: textValue("nonce"),
		ProviderJsapiPackage: textValue("prepay_id=wx-prepay"), ProviderJsapiSignType: textValue("RSA"), ProviderJsapiPaySign: textValue("signature"), ProviderJsapiExpiresAt: pgtype.Timestamptz{Time: now.Add(2 * time.Hour), Valid: true},
	}
	command := mapPayment(row)
	if command.State != orderport.EffectExecuted || command.JSAPIHandoff == nil || command.JSAPIHandoff.Package != "prepay_id=wx-prepay" || !command.JSAPIHandoff.ExpiresAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("mapped command = %+v", command)
	}
}
