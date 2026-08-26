package http

import (
	"html/template"
	"net/http"
	"strconv"

	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
)

func (handler *Handler) PublicProductDetailByPath(writer http.ResponseWriter, request *http.Request, rawKind, rawID string) {
	productID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || productID < 1 || strconv.FormatInt(productID, 10) != rawID {
		writeError(writer, request, sidebarapp.ErrNotFound)
		return
	}
	kind := sidebarapp.ShareableProductKind(rawKind)
	if kind != sidebarapp.ShareableProductOrdinary && kind != sidebarapp.ShareableProductServicePeriod {
		writeError(writer, request, sidebarapp.ErrNotFound)
		return
	}
	handler.PublicProductDetail(writer, request, kind, productID)
}

var publicProductTemplate = template.Must(template.New("product").Parse(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Name}}</title><style>body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;line-height:1.6;margin:0;background:#f5f6f8;color:#1f2329}main{max-width:680px;margin:40px auto;padding:32px;background:#fff;border-radius:12px}h1{margin-top:0}.meta{color:#646a73}.price{font-size:1.2rem;font-weight:600}</style><main><p class="meta">{{.KindLabel}} · CRM 本地商品详情</p><h1>{{.Name}}</h1><p class="price">{{.Currency}} {{.Price}}</p><p>{{.Description}}</p><p class="meta">商品编码：{{.ProductCode}} · 本地库存：{{.StockQuantity}}</p></main></html>`))

type publicProductView struct {
	KindLabel, ProductCode, Name, Description, Currency, Price string
	StockQuantity                                              int32
}

// PublicProductDetail renders only current local product fields. It never
// exposes a purchase, payment, entitlement, or customer-specific action.
func (handler *Handler) PublicProductDetail(writer http.ResponseWriter, request *http.Request, kind sidebarapp.ShareableProductKind, productID int64) {
	if handler == nil || handler.service == nil {
		writeError(writer, request, sidebarapp.ErrUnavailable)
		return
	}
	product, err := handler.service.PublicProduct(request.Context(), kind, productID)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	kindLabel := "普通商品"
	if product.Kind == sidebarapp.ShareableProductServicePeriod {
		kindLabel = "周期商品"
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err = publicProductTemplate.Execute(writer, publicProductView{
		KindLabel: kindLabel, ProductCode: product.ProductCode, Name: product.Name, Description: product.Description,
		Currency: product.Currency, Price: formatMinorPrice(product.PriceMinor), StockQuantity: product.StockQuantity,
	}); err != nil {
		return
	}
}
