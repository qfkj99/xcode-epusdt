# Docker 启动

已拉取本仓库代码时，在仓库根目录运行：

```bash
docker compose up -d
```

默认配置会从当前源码构建镜像、自动创建持久化目录并启动服务。首次构建需要下载依赖，后续构建使用缓存。

- 本机访问地址：`http://127.0.0.1:18080`
- 持久化目录：`/www/server/xcode-epusdt-data`
- 查看日志：`docker compose logs --tail=50 epusdt`
- 更新：`git pull --ff-only origin master` 后再次 `docker compose up -d`

域名反向代理、首次网页安装和备份步骤见 [宝塔 + Docker 部署](BAOTA_DOCKER.md)。默认采用 SQLite，无需额外部署 MySQL、Redis。
