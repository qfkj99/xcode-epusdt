package route

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/data"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/GMWalletApp/epusdt/model/request"
	"github.com/GMWalletApp/epusdt/model/service"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm/clause"
)

const epayQueryPath = "/payments/epay/v1/order/create-transaction/api.php"

type epayQueryResult struct {
	Code       int    `json:"code"`
	Status     int    `json:"status"`
	Money      string `json:"money"`
	TradeNo    string `json:"trade_no"`
	OutTradeNo string `json:"out_trade_no"`
	Type       string `json:"type"`
}

func epayQueryValues(orderID string) url.Values {
	return url.Values{"act": {"order"}, "pid": {"1"}, "key": {testAPIToken}, "out_trade_no": {orderID}}
}

func requestEPayQuery(t *testing.T, e *echo.Echo, method string, query, form url.Values) (*httptest.ResponseRecorder, epayQueryResult) {
	t.Helper()
	path := epayQueryPath
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	if method == http.MethodPost {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	}
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	t.Logf("%s api.php HTTP=%d body=%s", method, rec.Code, strings.TrimSpace(rec.Body.String()))
	var result epayQueryResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("query response is not EasyPay JSON: %v", err)
	}
	return rec, result
}

func seedEPayQueryOrder(t *testing.T, orderID string, mutate func(*mdb.Orders)) *mdb.Orders {
	t.Helper()
	key, err := data.GetEnabledApiKey("1")
	if err != nil || key == nil || key.ID == 0 {
		t.Fatalf("load merchant: %+v %v", key, err)
	}
	order := &mdb.Orders{
		TradeId: "trade-" + orderID, OrderId: orderID, ApiKeyID: key.ID,
		PaymentType: mdb.PaymentTypeEpay, EpayType: "usdt.tron", Name: "USDT top-up",
		Amount: 70.1234, ActualAmount: 10.0186, Currency: "CNY", Status: mdb.StatusPaySuccess,
		Network: mdb.NetworkTron, Token: "USDT",
	}
	if mutate != nil {
		mutate(order)
	}
	if err := dao.Mdb.Create(order).Error; err != nil {
		t.Fatal(err)
	}
	return order
}

func TestEPayQueryLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name     string
		selector string
		switchTo string
	}{
		{name: "tron", selector: "usdt.tron"},
		{name: "bsc", selector: "usdt.binance"},
		{name: "tron_to_bsc", selector: "usdt.tron", switchTo: mdb.NetworkBsc},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupTestEnv(t)
			if err := dao.Mdb.Clauses(clause.OnConflict{DoNothing: true}).Create(&mdb.ChainToken{
				Network: mdb.NetworkBsc, Symbol: "USDT", ContractAddress: "0x55d398326f99059fF775485246999027B3197955", Decimals: 18, Enabled: true,
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := dao.Mdb.Create(&mdb.WalletAddress{
				Network: mdb.NetworkBsc, Address: "0x1111111111111111111111111111111111111111", Status: mdb.TokenStatusEnable,
			}).Error; err != nil {
				t.Fatal(err)
			}
			orderID := "epay-query-" + tc.name
			parentTradeID := mustCreateEPayOrder(t, e, orderID, "https://93.184.216.34/return", url.Values{
				"type": {tc.selector}, "money": {"70.00"},
			})
			values := epayQueryValues(orderID)
			// Sub2API sends identical query and body parameters when confirming callbacks.
			rec, pending := requestEPayQuery(t, e, http.MethodPost, values, values)
			if rec.Code != http.StatusOK || pending.Code != 1 || pending.Status != 0 || pending.Money != "70.0000" || pending.TradeNo != parentTradeID || pending.OutTradeNo != orderID {
				t.Fatalf("pending query is incorrect: HTTP=%d %+v", rec.Code, pending)
			}
			settlementTradeID := parentTradeID
			if tc.switchTo != "" {
				child, err := service.SwitchNetwork(&request.SwitchNetworkRequest{TradeId: parentTradeID, Token: "USDT", Network: tc.switchTo})
				if err != nil {
					t.Fatal(err)
				}
				settlementTradeID = child.TradeId
				if settlementTradeID == parentTradeID {
					t.Fatal("switch did not create a child order")
				}
			}
			// Exercise the real local settlement path with a simulated chain event;
			// no external RPC or blockchain transaction is performed by this test.
			if err := service.OrderProcessing(&request.OrderProcessingRequest{
				TradeId: settlementTradeID, BlockTransactionId: "simulated-chain-event-" + tc.name,
			}); err != nil {
				t.Fatal(err)
			}
			rec, paid := requestEPayQuery(t, e, http.MethodPost, values, values)
			parent, err := data.GetOrderInfoByTradeId(parentTradeID)
			if err != nil {
				t.Fatal(err)
			}
			key, err := data.GetEnabledApiKey("1")
			if err != nil {
				t.Fatal(err)
			}
			callback, err := service.BuildEPayResultParams(parent, key)
			if err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || paid.Code != 1 || paid.Status != 1 || paid.Money != callback["money"] || paid.TradeNo != callback["trade_no"] || paid.OutTradeNo != callback["out_trade_no"] || paid.Type != callback["type"] {
				t.Fatalf("query differs from paid callback: HTTP=%d query=%+v callback=%v", rec.Code, paid, callback)
			}
			if paid.Money != "70.0000" || paid.TradeNo != parentTradeID || paid.Type != tc.selector || parent.ActualAmount == parent.Amount {
				t.Fatalf("query lost original fiat amount, channel or parent identity: %+v parent=%+v", paid, parent)
			}
			if tc.switchTo != "" && parent.PayBySubId == 0 {
				t.Fatal("parent was not settled through its child")
			}
			if err := service.OrderProcessing(&request.OrderProcessingRequest{TradeId: settlementTradeID, BlockTransactionId: "simulated-chain-event-" + tc.name}); err == nil {
				t.Fatal("duplicate settlement unexpectedly succeeded")
			}
			_, repeated := requestEPayQuery(t, e, http.MethodGet, values, nil)
			if repeated != paid {
				t.Fatalf("repeated query changed paid result: before=%+v after=%+v", paid, repeated)
			}
		})
	}
}

func TestEPayQueryMethodsAndStates(t *testing.T) {
	e := setupTestEnv(t)
	for _, state := range []int{mdb.StatusWaitPay, mdb.StatusPaySuccess, mdb.StatusExpired, mdb.StatusWaitSelect, 99} {
		order := seedEPayQueryOrder(t, fmt.Sprintf("query-state-%d", state), func(o *mdb.Orders) { o.Status = state })
		values := epayQueryValues(order.OrderId)
		for _, mode := range []string{"get", "post_body", "post_query", "post_both", "trade_no"} {
			t.Run(fmt.Sprintf("state_%d/%s", state, mode), func(t *testing.T) {
				method, query, form := http.MethodGet, values, url.Values(nil)
				switch mode {
				case "post_body":
					method, query, form = http.MethodPost, nil, values
				case "post_query":
					method = http.MethodPost
				case "post_both":
					method, form = http.MethodPost, values
				case "trade_no":
					query = epayQueryValues("")
					query.Del("out_trade_no")
					query.Set("trade_no", order.TradeId)
				}
				rec, result := requestEPayQuery(t, e, method, query, form)
				wantStatus := 0
				if state == mdb.StatusPaySuccess {
					wantStatus = 1
				}
				if rec.Code != http.StatusOK || result.Code != 1 || result.Status != wantStatus || result.Money != "70.1234" || result.TradeNo != order.TradeId || result.OutTradeNo != order.OrderId || result.Type != "usdt.tron" {
					t.Fatalf("incorrect query response: HTTP=%d %+v", rec.Code, result)
				}
			})
		}
	}
}

func TestEPayQueryRejectsUnauthorizedAndUnrelatedOrders(t *testing.T) {
	e := setupTestEnv(t)
	owned := seedEPayQueryOrder(t, "owned", nil)
	otherKey, err := data.GetEnabledApiKey(testAPIToken)
	if err != nil {
		t.Fatal(err)
	}
	seedEPayQueryOrder(t, "other-merchant", func(o *mdb.Orders) { o.ApiKeyID = otherKey.ID })
	seedEPayQueryOrder(t, "unowned-legacy", func(o *mdb.Orders) { o.ApiKeyID = 0 })
	seedEPayQueryOrder(t, "gmpay", func(o *mdb.Orders) { o.PaymentType = mdb.PaymentTypeGmpay })
	child := seedEPayQueryOrder(t, "child", func(o *mdb.Orders) { o.ParentTradeId = owned.TradeId })
	for _, tc := range []struct {
		name string
		edit func(url.Values)
	}{
		{"missing_key", func(v url.Values) { v.Del("key") }},
		{"wrong_key", func(v url.Values) { v.Set("key", "wrong-secret") }},
		{"missing_pid", func(v url.Values) { v.Del("pid") }},
		{"unknown_pid", func(v url.Values) { v.Set("pid", "99999") }},
		{"other_merchant", func(v url.Values) { v.Set("out_trade_no", "other-merchant") }},
		{"unowned_legacy", func(v url.Values) { v.Set("out_trade_no", "unowned-legacy") }},
		{"gmpay_order", func(v url.Values) { v.Set("out_trade_no", "gmpay") }},
		{"child_order", func(v url.Values) { v.Set("out_trade_no", "child") }},
		{"child_trade", func(v url.Values) { v.Del("out_trade_no"); v.Set("trade_no", child.TradeId) }},
		{"missing_order", func(v url.Values) { v.Set("out_trade_no", "missing") }},
		{"missing_identifiers", func(v url.Values) { v.Del("out_trade_no") }},
		{"conflicting_identifiers", func(v url.Values) { v.Set("trade_no", child.TradeId) }},
		{"unsupported_refund", func(v url.Values) { v.Set("act", "refund") }},
		{"missing_action", func(v url.Values) { v.Del("act") }},
		{"sql_like_order", func(v url.Values) { v.Set("out_trade_no", "' OR 1=1 --") }},
		{"duplicate_key", func(v url.Values) { v.Add("key", "wrong-secret") }},
		{"duplicate_order", func(v url.Values) { v.Add("out_trade_no", "other-merchant") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := epayQueryValues(owned.OrderId)
			tc.edit(values)
			rec, result := requestEPayQuery(t, e, http.MethodGet, values, nil)
			assertEPayQueryRejected(t, rec, result)
		})
	}
	key, err := data.GetEnabledApiKey("1")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("disabled_merchant", func(t *testing.T) {
		if err := dao.Mdb.Model(key).Update("status", mdb.ApiKeyStatusDisable).Error; err != nil {
			t.Fatal(err)
		}
		rec, result := requestEPayQuery(t, e, http.MethodPost, nil, epayQueryValues(owned.OrderId))
		assertEPayQueryRejected(t, rec, result)
		if err := dao.Mdb.Model(key).Update("status", mdb.ApiKeyStatusEnable).Error; err != nil {
			t.Fatal(err)
		}
	})
	t.Run("ip_whitelist", func(t *testing.T) {
		if err := dao.Mdb.Model(key).Update("ip_whitelist", "198.51.100.0/24").Error; err != nil {
			t.Fatal(err)
		}
		rec, result := requestEPayQuery(t, e, http.MethodGet, epayQueryValues(owned.OrderId), nil)
		assertEPayQueryRejected(t, rec, result)
		if err := dao.Mdb.Model(key).Update("ip_whitelist", "192.0.2.0/24").Error; err != nil {
			t.Fatal(err)
		}
		rec, result = requestEPayQuery(t, e, http.MethodGet, epayQueryValues(owned.OrderId), nil)
		if rec.Code != http.StatusOK || result.Code != 1 {
			t.Fatalf("allowed client rejected: HTTP=%d %+v", rec.Code, result)
		}
	})
}

func assertEPayQueryRejected(t *testing.T, rec *httptest.ResponseRecorder, result epayQueryResult) {
	t.Helper()
	if rec.Code == http.StatusNotFound || result.Code == 1 || result.Status == 1 || result.Money != "" || result.TradeNo != "" || result.OutTradeNo != "" {
		t.Fatalf("request must return an explicit failure without order data: HTTP=%d %+v", rec.Code, result)
	}
	if strings.Contains(rec.Body.String(), testAPIToken) {
		t.Fatal("failure response exposed the merchant secret")
	}
}

func TestEPayQueryRejectsConflictingQueryAndBody(t *testing.T) {
	e := setupTestEnv(t)
	order := seedEPayQueryOrder(t, "conflict", nil)
	for _, field := range []string{"pid", "key", "act", "out_trade_no", "trade_no"} {
		t.Run(field, func(t *testing.T) {
			query, form := epayQueryValues(order.OrderId), epayQueryValues(order.OrderId)
			if field == "trade_no" {
				query.Set(field, order.TradeId)
			}
			form.Set(field, "conflicting-value")
			rec, result := requestEPayQuery(t, e, http.MethodPost, query, form)
			assertEPayQueryRejected(t, rec, result)
		})
	}
}
