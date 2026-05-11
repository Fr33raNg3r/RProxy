# ============================================================================
# TProxy Server Nginx 配置模板
# 安装时由 install.sh 渲染，输出到 /etc/nginx/conf.d/tproxy-server.conf
# 占位符：
#   {{DOMAIN}}     - 用户输入的域名
#   {{WS_PATH}}    - 随机生成的 WebSocket 路径（如 /a1b2c3d4）
#   {{WEB_ROOT}}   - 静态网站根目录（如 /var/www/tproxy-server）
#   {{SSL_DIR}}    - 证书目录
#   {{XRAY_PORT}}  - Xray 监听的本地端口（默认 9890）
#   {{PHP_SOCK}}   - PHP-FPM socket 路径
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

# ----- 与 Xray 内部端口的 upstream（启用 keepalive）-----
upstream xray_ws_backend {
    server 127.0.0.1:{{XRAY_PORT}};
    keepalive 32;
}

# ----- 443 端口：TLS 终结 + WS 反代 + LibreSpeed -----
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

    # 安全头
    add_header Strict-Transport-Security "max-age=63072000" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;

    # 真网站根目录
    root {{WEB_ROOT}};
    index index.html index.php;
    charset utf-8;

    # ----- WebSocket 反代到 Xray（仅 Upgrade 请求）-----
    # 关键：if 检查 $http_upgrade，非 WebSocket 请求 404，看起来更像普通 API
    location = {{WS_PATH}} {
        if ($http_upgrade != "websocket") {
            return 404;
        }
        proxy_pass http://xray_ws_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        # WS 长连接：禁掉读超时（默认 60s 会切断长连接）
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
        proxy_buffering off;
    }

    # ----- LibreSpeed 后端（PHP）-----
    location ~ \.php$ {
        try_files $uri =404;
        fastcgi_pass unix:{{PHP_SOCK}};
        fastcgi_index index.php;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
        # LibreSpeed 大数据量上传用
        client_max_body_size 100M;
        fastcgi_read_timeout 300s;
    }

    # ----- 真网站：静态文件 -----
    location / {
        try_files $uri $uri/ /index.html;
    }

    # ----- ACME 续期验证（同时让 acme.sh webroot 也走 HTTPS）-----
    location /.well-known/acme-challenge/ {
        root {{WEB_ROOT}};
    }

    # ----- 隐藏 nginx 版本 -----
    server_tokens off;
}
