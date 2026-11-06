# 易支付查单与 Sub2API 对接

此适配仅修改 epusdt。对应 ranxi2001/sub2api 的易支付跳转收银台模式；核对版本为 production 提交 `511e40d17049f0d1e7a3ff7c3ad0c0cae6304c91`。不使用 BEpusdt 外接网关模式。

## 后台配置

先部署包含本适配和金额锁修复的 epusdt，配置并启用 TRON、BSC 的 USDT 钱包、节点及汇率。在 API 密钥管理中取得实际的数字 PID 和对应 secret_key。

| Sub2API 配置项 | 内容 |
| --- | --- |
| 支付类型 | 易支付 |
| 支付模式 | 跳转收银台（popup） |
| 网关地址 apiBase | `https://pay.example.com/payments/epay/v1/order/create-transaction` |
| PID / PKey | epusdt 后台实际 PID / secret_key |
| TRON 自定义渠道 | 本地类型 `usdt_tron`，上游类型 `usdt.tron` |
| BSC 自定义渠道 | 本地类型 `usdt_bsc`，上游类型 `usdt.binance` |
| 回调、返回基础地址 | Sub2API 自己的公网 HTTPS 地址 |

自定义渠道名称可设为 `USDT (TRC20)`、`USDT (BEP20)`。默认易支付 alipay/wxpay 按需关闭，避免误选。

核对 epusdt 的 `epay.default_currency` 与 Sub2API 订单计价货币一致；未指定时 epusdt 默认 CNY。`money` 是订单原始法币金额，收银台显示的精确 USDT 金额由汇率及金额分配决定。

跳转收银台的 submit.php 请求由客户浏览器发出，查单由 Sub2API 服务器发出。配置 API 密钥 IP 白名单时需考虑两者；只允许 Sub2API 服务器 IP 会拦截客户进入收银台。接口仍验证商户密钥/签名。

## 查单协议

`GET` 或表单 `POST /payments/epay/v1/order/create-transaction/api.php`

参数：`act=order`、`pid`、`key`、`out_trade_no`。也支持用 `trade_no` 查网关订单；同时传入两个编号时必须对应同一订单。查询字符串与表单中的同名参数必须一致。

成功响应示例（已支付）：

```json
{"code":1,"msg":"success","pid":"1000","trade_no":"gateway-order-id","out_trade_no":"merchant-order-id","type":"usdt.tron","name":"充值","money":"70.0000","status":1}
```

只有内部已支付状态映射为 `status:1`；待支付、待选网络、过期及其他状态均为 `0`。金额与已签名回调一致，使用原始订单 Amount，保留四位小数。切链后查原始父订单，成功响应保留商户收到的父单编号和渠道类型。

仅启用的商户凭据可查询其自己的易支付父订单；内部子单、其他商户订单、GMPay 订单不会暴露。不存在/无权限、参数错误、未支持的操作返回非成功 `code`。查询不改变付款状态，不触发到账、不补发回调、不释放金额锁。

## 确认流程与支持范围

Sub2API 收到签名回调后会再查单，确认已付款、订单编号和原始金额匹配后继续入账。部署后先做一笔小额 TRON 和一笔 BSC 实付，确认 epusdt 已支付、回调成功以及 Sub2API 实际余额增长；本仓库自动测试使用本地模拟结算，不能代替链上验收。

本适配不提供 `mapi.php` 二维码下单，也不提供自动退款；`act=refund` 会明确返回错误，不能配置为在线退款网关。

查单使用 HTTPS。Sub2API 会把 key 同时放在查询字符串和表单中；epusdt 内置访问日志已对该接口隐藏整个查询字符串。反向代理/CDN 日志也应对该路径去除查询字符串（例如 Nginx 使用不含参数的 `$uri`），不要记录 `$request` 或 `$request_uri` 中的密钥。不要把真实 key 放进公开链接、工单或终端截图。
