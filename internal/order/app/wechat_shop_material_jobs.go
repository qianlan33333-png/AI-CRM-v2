package app

const WeChatShopMaterialSyncJobKind = "order_wechat_shop_material_sync"

type WeChatShopMaterialSyncArgs struct {
	ProviderOrderID string `json:"provider_order_id"`
}

func (WeChatShopMaterialSyncArgs) Kind() string { return WeChatShopMaterialSyncJobKind }
