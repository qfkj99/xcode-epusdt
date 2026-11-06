# 使用已拉取的源码部署

源码放到 `/www/wwwroot/xcode-epusdt`。此流程使用同目录的 `compose.baota.yaml`，构建上下文为 `.`；配置和两份数据库保存到 `/www/server/xcode-epusdt-data`。宝塔收款域名使用整站反代，不使用源码目录作为公开静态文件站点。

## 构建并启动

`compose.baota.yaml` 已包含在本仓库，拉取代码后直接使用，无需另外上传文件。已有仓库先执行 `git pull --ff-only origin master`，然后检查：

```bash
cd /www/wwwroot/xcode-epusdt
git status --short
git log -1 --oneline
git merge-base --is-ancestor 669af85d917a6d85577e59860cd911243dbb226d HEAD
```

最后一条无输出且退出码为 0 表示已包含本轮修复。若提示 unknown revision、not a git repository，或退出码非零，先核对源码来源；不要继续构建标注为本修复版本的镜像。此检查不修改已拉取的文件。

```bash
mkdir -p /www/server/xcode-epusdt-data
chmod 700 /www/server/xcode-epusdt-data
docker compose -f compose.baota.yaml config --quiet
docker compose -f compose.baota.yaml build
docker compose -f compose.baota.yaml up -d --pull never
docker compose -f compose.baota.yaml ps
docker compose -f compose.baota.yaml logs --tail=50 epusdt
```

所有管理命令都带 `-f compose.baota.yaml`，明确选择本次配置。已有源文件无需再放进 source 子目录。此处按尚未初始化的新部署编写；若已用之前的 `/opt/xcode-epusdt/data` 初始化，请继续挂载原数据路径，勿直接换成空目录。

## 首次初始化

先检查：

```bash
curl -fsS http://127.0.0.1:18080/api/install/defaults
```

将以下域名换成真实收款域名，只在首次安装时提交：

```bash
curl -fsS -X POST http://127.0.0.1:18080/api/install \
  -H 'Content-Type: application/json' \
  --data-binary @- <<'JSON'
{
  "app_name": "epusdt",
  "app_uri": "https://pay.example.com",
  "http_bind_addr": "0.0.0.0",
  "http_bind_port": 8000,
  "runtime_root_path": "/app/conf/runtime",
  "log_save_path": "/app/conf/logs",
  "order_expiration_time": 10,
  "order_notice_max_retry": 10
}
JSON
```

保存响应中的 init_password，用户名 admin。之后使用宝塔 Nginx 将收款域名整站反向代理到 `http://127.0.0.1:18080`，关闭缓存并配置 HTTPS。后台登录路径为 `/sign-in`；代理需覆盖 `.php` 路由并避免在访问日志中记录查单密钥。公网无需开放 18080。

查单路由检查：

```bash
curl -fsS 'http://127.0.0.1:18080/payments/epay/v1/order/create-transaction/api.php?act=order'
```

预期 `{"code":-1,"msg":"Invalid merchant credentials"}`，说明新接口存在。钱包、汇率及 Sub2API 参数见 [易支付查单与 Sub2API 对接](epay-sub2api.md)。TRON 和 BSC 各做一笔小额实付，确认钱包到账、epusdt 已支付及回调成功、Sub2API 实际余额增加。

## 数据与备份

- 配置：`/www/server/xcode-epusdt-data/.env`
- 主库：`/www/server/xcode-epusdt-data/epusdt.db`
- 运行库：`/www/server/xcode-epusdt-data/runtime/epusdt-runtime.db`
- 日志：`/www/server/xcode-epusdt-data/logs/`

备份时短暂停服务，保存完整数据目录：

```bash
cd /www/wwwroot/xcode-epusdt
mkdir -p /www/backup/xcode-epusdt
chmod 700 /www/backup/xcode-epusdt
docker compose -f compose.baota.yaml stop epusdt
tar -C /www/server -czf "/www/backup/xcode-epusdt/data-$(date +%Y%m%d-%H%M%S).tar.gz" xcode-epusdt-data
docker compose -f compose.baota.yaml start epusdt
```

Compose 从当前仓库源码构建本地镜像 `xcode-epusdt:local`，版本标识为 `baota-local`；每次更新源码后重新执行 build/up。源代码版本以 `git rev-parse HEAD` 为准。
