# 宝塔 + Docker 部署

源码已经拉取到 `/www/wwwroot/xcode-epusdt` 时，直接执行：

```bash
cd /www/wwwroot/xcode-epusdt
git pull --ff-only origin master
docker compose up -d
```

默认 `docker-compose.yaml` 会自动从当前源码构建镜像、创建数据目录并启动服务。首次需要下载构建镜像和 Go 依赖，等待命令完成即可；后续使用 Docker 构建缓存。默认采用 SQLite，无需额外安装 MySQL、Redis。

应用只监听宿主机 `127.0.0.1:18080`，配置、订单库和运行库保存在 `/www/server/xcode-epusdt-data`。之前使用 `compose.baota.yaml` 的部署也可以直接使用上面的命令，数据路径保持一致。

## 宝塔配置域名与 HTTPS

将收款域名解析到服务器，在宝塔创建整站反向代理并申请 SSL 证书：

| 配置项 | 内容 |
| --- | --- |
| 收款域名 | 例如 `pay.example.com` |
| 代理目录 | `/` |
| 目标 URL | `http://127.0.0.1:18080` |
| 发送域名 / Host | `$host` 或真实收款域名 |
| 缓存 | 关闭 |
| SSL | 启用 HTTPS |

使用反向代理站点，不要把源码作为公开静态网站目录。`submit.php`、`api.php` 是 Go 应用路由，需要一并转发。反向代理访问日志应隐藏查询字符串或关闭该代理位置的访问日志，避免记录查单 URL 中的商户密钥。

以上适用于宿主机运行的宝塔 Nginx；公网只需开放 80、443，无需开放 18080。

## 首次在网页安装

访问 `https://pay.example.com/install`，把域名换成自己的，在安装页面填写：

| 字段 | 值 |
| --- | --- |
| 应用地址 / app_uri | `https://pay.example.com` |
| HTTP 绑定地址 | **`0.0.0.0`** |
| HTTP 端口 | `8000` |
| 运行目录 | `/app/conf/runtime` |
| 日志目录 | `/app/conf/logs` |
| 订单有效期 | `10` 分钟 |
| 回调重试上限 | `10` |

容器内必须监听 `0.0.0.0:8000`。安装成功后保存页面显示的初始密码，用户名是 `admin`，后台登录地址为 `/sign-in`。登录后修改密码；后续重启会直接使用已保存的配置。

钱包、汇率及 Sub2API 参数见 [易支付查单与 Sub2API 对接](epay-sub2api.md)。TRON 和 BSC 各做一笔小额实付，确认钱包到账、epusdt 已支付及回调成功、Sub2API 实际余额增加。

## 更新与常用命令

更新源码后，仍然用同一条启动命令，Compose 会按当前源码重新构建：

```bash
git pull --ff-only origin master
docker compose up -d
```

查看状态与日志：

```bash
docker compose ps
docker compose logs --tail=50 epusdt
```

服务重启使用 `docker compose restart epusdt`。

## 数据与备份

- 配置：`/www/server/xcode-epusdt-data/.env`
- 主库：`/www/server/xcode-epusdt-data/epusdt.db`
- 运行库：`/www/server/xcode-epusdt-data/runtime/epusdt-runtime.db`
- 日志：`/www/server/xcode-epusdt-data/logs/`

若已在其他目录初始化（例如 `/opt/xcode-epusdt/data`），在仓库根目录的 Compose `.env` 中设置 `EPUSDT_DATA_DIR=/opt/xcode-epusdt/data`，继续使用原数据目录。自定义宿主端口可在同一文件设置 `EPUSDT_PORT=18081`，并同步修改宝塔代理目标。默认部署不需要创建这个文件。

备份时短暂停服务并保存整个数据目录，包含 SQLite WAL 文件：

```bash
cd /www/wwwroot/xcode-epusdt
mkdir -p /www/backup/xcode-epusdt
chmod 700 /www/backup/xcode-epusdt
docker compose stop epusdt
tar -C /www/server -czf "/www/backup/xcode-epusdt/data-$(date +%Y%m%d-%H%M%S).tar.gz" xcode-epusdt-data
docker compose start epusdt
```

若使用自定义数据目录，备份对应目录。将备份另存到独立位置。
