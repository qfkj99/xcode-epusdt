package comm

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"github.com/GMWalletApp/epusdt/middleware"
	"github.com/GMWalletApp/epusdt/model/data"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/labstack/echo/v4"
)

// QueryEPayOrder implements the legacy api.php?act=order contract. Its status
// values are EasyPay's (0 unpaid, 1 paid), not the internal order status enum.
func (c *BaseCommController) QueryEPayOrder(ctx echo.Context) error {
	ctx.Response().Header().Set("Cache-Control", "no-store")
	ctx.Response().Header().Set("Referrer-Policy", "no-referrer")
	fail := func(status int, message string) error {
		return ctx.JSON(status, map[string]interface{}{"code": -1, "msg": message})
	}

	// ParseForm merges URL and form-body parameters. Accept repeated values
	// only when identical: clients may send the same credentials in both places.
	// Conflicting values must never select a different merchant or order.
	req := ctx.Request()
	if err := req.ParseForm(); err != nil {
		return fail(http.StatusOK, "Invalid parameters")
	}
	params := make(map[string]string)
	for _, key := range []string{"act", "pid", "key", "out_trade_no", "trade_no"} {
		values := req.Form[key]
		if len(values) == 0 {
			continue
		}
		for _, value := range values[1:] {
			if value != values[0] {
				return fail(http.StatusOK, "Conflicting parameters")
			}
		}
		params[key] = values[0]
	}
	if params["act"] != "order" {
		return fail(http.StatusOK, "Unsupported action")
	}
	pid := strings.TrimSpace(params["pid"])
	if pid == "" || params["key"] == "" {
		return fail(http.StatusOK, "Invalid merchant credentials")
	}
	apiKey, err := data.GetEnabledApiKey(pid)
	if err != nil {
		return fail(http.StatusInternalServerError, "Unable to query order")
	}
	if apiKey == nil || apiKey.ID == 0 || apiKey.SecretKey == "" ||
		subtle.ConstantTimeCompare([]byte(apiKey.SecretKey), []byte(params["key"])) != 1 ||
		!middleware.IsIPWhitelisted(apiKey.IpWhitelist, ctx.RealIP()) {
		return fail(http.StatusOK, "Invalid merchant credentials")
	}

	outTradeNo := params["out_trade_no"]
	tradeNo := params["trade_no"]
	if strings.TrimSpace(outTradeNo) == "" && strings.TrimSpace(tradeNo) == "" {
		return fail(http.StatusOK, "Missing order identifier")
	}
	order, err := data.GetEPayOrderForMerchant(apiKey.ID, outTradeNo, tradeNo)
	if err != nil {
		return fail(http.StatusInternalServerError, "Unable to query order")
	}
	if order == nil || order.ID == 0 {
		return fail(http.StatusOK, "Order not found")
	}
	_ = data.TouchApiKeyUsage(apiKey.ID)

	status := 0
	if order.Status == mdb.StatusPaySuccess {
		status = 1
	}
	epayType := strings.TrimSpace(order.EpayType)
	if epayType == "" {
		epayType = "alipay"
	}
	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"code":         1,
		"msg":          "success",
		"pid":          apiKey.Pid,
		"trade_no":     order.TradeId,
		"out_trade_no": order.OrderId,
		"type":         epayType,
		"name":         order.Name,
		// Keep the merchant's fiat amount and precision identical to the
		// callback; ActualAmount is the on-chain amount and may be adjusted.
		"money":  fmt.Sprintf("%.4f", order.Amount),
		"status": status,
	})
}
