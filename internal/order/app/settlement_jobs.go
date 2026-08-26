package app

const (
	PaymentEffectBridgeJobKind       = "pe01_wechat_payment_effect"
	RefundEffectBridgeJobKind        = "pe01_wechat_refund_effect"
	PaymentReconcileJobKind          = "pe01_wechat_payment_reconcile"
	RefundReconcileJobKind           = "pe01_wechat_refund_reconcile"
	WeChatShopRefundJobKind          = "order_wechat_shop_refund"
	WeChatShopRefundReconcileJobKind = "order_wechat_shop_refund_reconcile"
)

type PaymentEffectBridgeArgs struct {
	CommandID int64 `json:"command_id"`
}

func (PaymentEffectBridgeArgs) Kind() string { return PaymentEffectBridgeJobKind }

type RefundEffectBridgeArgs struct {
	RefundID int64 `json:"refund_id"`
}

func (RefundEffectBridgeArgs) Kind() string { return RefundEffectBridgeJobKind }

type PaymentReconcileArgs struct {
	CommandID int64 `json:"command_id"`
}

func (PaymentReconcileArgs) Kind() string { return PaymentReconcileJobKind }

type RefundReconcileArgs struct {
	RefundID int64 `json:"refund_id"`
}

func (RefundReconcileArgs) Kind() string { return RefundReconcileJobKind }

type WeChatShopRefundArgs struct {
	RefundID int64 `json:"refund_id"`
}

func (WeChatShopRefundArgs) Kind() string { return WeChatShopRefundJobKind }

type WeChatShopRefundReconcileArgs struct {
	RefundID int64 `json:"refund_id"`
}

func (WeChatShopRefundReconcileArgs) Kind() string { return WeChatShopRefundReconcileJobKind }
