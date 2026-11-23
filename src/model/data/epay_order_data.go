package data

import (
	"strings"

	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/mdb"
)

// GetEPayOrderForMerchant only exposes merchant-facing EPay parent orders.
// Network-switch children have internal identities and never send callbacks.
// When both identifiers are supplied they must identify the same order.
func GetEPayOrderForMerchant(apiKeyID uint64, outTradeNo, tradeNo string) (*mdb.Orders, error) {
	order := new(mdb.Orders)
	if apiKeyID == 0 || (outTradeNo == "" && tradeNo == "") {
		return order, nil
	}
	query := dao.Mdb.Model(order).
		Where("api_key_id = ?", apiKeyID).
		Where("LOWER(payment_type) = ?", strings.ToLower(mdb.PaymentTypeEpay)).
		Where("parent_trade_id = ?", "")
	if outTradeNo != "" {
		query = query.Where("order_id = ?", outTradeNo)
	}
	if tradeNo != "" {
		query = query.Where("trade_id = ?", tradeNo)
	}
	err := query.Limit(1).Find(order).Error
	return order, err
}
