# ============================================================================
# RProxy Server Nginx 配置模板
# 安装时由 install.sh 渲染，输出到 /etc/nginx/conf.d/rproxy-server.conf
# 占位符：
#   {{DOMAIN}}     - 用户输入的域名
#   {{WS_PATH}}    - 随机生成的 XHTTP 路径（如 /a1b2c3d4；沿用旧字段名 ws_path）
#   {{WEB_ROOT}}   - 静态网站根目录（如 /var/www/rproxy-server）
#   {{SSL_DIR}}    - 证书目录
# XHTTP 的 9891 端口直接写死（须与 common.sh 的 XRAY_XHTTP_PORT 一致）：
# 升级时可能由本机旧 install.sh 渲染新模板，新增占位符会无人替换导致 nginx -t 失败
#
# 约束：本机可能还有用户自己的站点（如 Docker 版 Vaultwarden 经 conf.d 下另一个文件反代）。
# 所有指令只写在下面的 server 块内，不写 http 层，避免影响别的站点。
# ============================================================================

# ----- 80 端口：跳转到 HTTPS + 保留 ACME 验证 -----
server {
    listen 80;
    listen [::]:80;
    server_name {{DOMAIN}};

    # acme.sh webroot 续期验证用
    location /.well-known/acme-challenge/ {
        root {{WEB_ROOT}};
    }

    location / {
        return 301 https://$host$request_uri;
    }
}

# ----- 443 端口：TLS 终结 + XHTTP 反代 + 静态网站 -----
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name {{DOMAIN}};

    # ----- TLS 配置 -----
    ssl_certificate     {{SSL_DIR}}/server.crt;
    ssl_certificate_key {{SSL_DIR}}/server.key;
    ssl_session_timeout 1d;
    ssl_session_cache   shared:SSL:50m;
    ssl_session_tickets off;
    ssl_protocols       TLSv1.3;
    ssl_prefer_server_ciphers off;

    # 安全头（location 内不要再写 add_header，否则这些头不会被继承）
    add_header Strict-Transport-Security "max-age=63072000" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Robots-Tag "none" always;

    root {{WEB_ROOT}};
    index index.html;
    charset utf-8;
    server_tokens off;

    # 伪装站不记访问日志，减少磁盘写入
    access_log off;

    # 伪装站不接收大请求体；只有下面的 XHTTP 路径放开
    client_max_body_size 1m;

    # 所有 404 都显示同一个页面（与 404.html 同源，XHTTP 路径上被改写的 400 也走这里）
    error_page 404 /404.html;
    location = /404.html {
        internal;
    }
    location @site_404 {
        return 404;
    }

    # ----- VLESS + XHTTP 反代到 Xray（h2c，写法同 Xray 官方示例）-----
    # XHTTP 请求总以 {{WS_PATH}}/ 开头
    # 不传 X-Forwarded-For：新版 Xray 未配置 trustedXForwardedFor 时会对每个连接打警告
    # Xray 对非 XHTTP 请求会回 400（带 x-padding 等特征头）：这里把 400/404/405 改写成站点自己的 404 页面。
    # 只改写这几个状态码；502/504（Xray 未运行或卡死）原样保留，watchdog 靠它们判断 Xray 是否存活
    location {{WS_PATH}}/ {
        client_max_body_size 0;
        client_body_timeout 5m;
        grpc_read_timeout 315;
        grpc_send_timeout 5m;
        grpc_pass grpc://127.0.0.1:9891;
        grpc_intercept_errors on;
        error_page 400 404 405 = @site_404;
    }

    # 不带结尾斜杠的路径：不让 nginx 自动 301 到 {{WS_PATH}}/（那会暴露该路径存在）
    location = {{WS_PATH}} {
        return 404;
    }

    # ----- 登录接口：没有真实账号，一律 401 -----
    location = /api/auth/login {
        default_type application/json;
        return 401 '{"error":"invalid_credentials"}';
    }
    location /api/ {
        default_type application/json;
        return 401 '{"error":"unauthorized"}';
    }

    # ----- ACME 续期验证（同时让 acme.sh webroot 也走 HTTPS）-----
    location /.well-known/acme-challenge/ {
        root {{WEB_ROOT}};
    }

    # ----- 静态网站 -----
    location /posters/ {
        expires 7d;
        try_files $uri =404;
    }

    location / {
        try_files $uri $uri/ =404;
    }
}
