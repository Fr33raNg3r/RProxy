# ============================================================================
# mosdns v5 配置文件
# 路径：/opt/tproxy-gw/config/mosdns/config.yaml
# 用途：DNS 分流 - 国内域名走国内 DNS，国外域名走 DoH（经 Xray 出境）
#       国内域名解析得到的 IP 自动写入 nftables cn_ips 集合
# ============================================================================

log:
  level: info
  file: "/opt/tproxy-gw/logs/mosdns.log"

api:
  http: "127.0.0.1:9091"

plugins:
  # ----------- 缓存 -----------
  - tag: cache
    type: cache
    args:
      size: 8192
      lazy_cache_ttl: 86400

  # ----------- 国内 DNS 上游 -----------
  - tag: forward_local
    type: forward
    args:
      concurrent: 2
      upstreams:
        - addr: "udp://223.5.5.5"
          enable_pipeline: true
        - addr: "udp://119.29.29.29"
          enable_pipeline: true

  # ----------- 国外 DoH 上游（流量将被 nftables 劫持到 Xray 出境）-----------
  - tag: forward_remote
    type: forward
    args:
      concurrent: 2
      upstreams:
        - addr: "https://1.1.1.1/dns-query"
        - addr: "https://8.8.8.8/dns-query"

  # ----------- 域名集合：国内 -----------
  - tag: geosite_cn
    type: domain_set
    args:
      files:
        - "/opt/tproxy-gw/data/geo/geosite-cn.txt"

  # ----------- 域名集合：国外（gfw + geolocation-!cn）-----------
  - tag: geosite_no_cn
    type: domain_set
    args:
      files:
        - "/opt/tproxy-gw/data/geo/geosite-no-cn.txt"

  # ----------- 用户黑名单（强制走代理）-----------
  - tag: user_blacklist
    type: domain_set
    args:
      files:
        - "/opt/tproxy-gw/config/dns/blacklist.txt"

  # ----------- 用户白名单（强制直连）-----------
  - tag: user_whitelist
    type: domain_set
    args:
      files:
        - "/opt/tproxy-gw/config/dns/whitelist.txt"

  # ----------- 用户静态 hosts -----------
  - tag: user_hosts
    type: hosts
    args:
      files:
        - "/opt/tproxy-gw/config/dns/hosts.txt"

  # ----------- IP 集合：国内（用于把解析结果写到 nftables cn_ips）-----------
  - tag: geoip_cn
    type: ip_set
    args:
      files:
        - "/opt/tproxy-gw/data/geo/geoip-cn.txt"

  # ============================================================================
  # 子序列：必须定义在 main_sequence 之前（mosdns v5 不支持向前引用）
  # ============================================================================

  # ----------- 子序列：把响应中的国内 IP 加入 cn_ips set -----------
  # mosdns v5 的 nftset 不是独立插件，是 sequence 内置 action
  # 参数格式：nftset 表族,表名,集合名,IP类型,掩码
  - tag: add_to_cn_set
    type: sequence
    args:
      - matches:
          - resp_ip $geoip_cn
        exec: nftset inet,tp,cn_ips,ipv4_addr,32
      - exec: accept

  # ----------- 兜底：未识别域名走"本地优先 + 远程兜底"策略 -----------
  - tag: primary_local_secondary_remote
    type: sequence
    args:
      - exec: $forward_local
      - matches:
          - resp_ip $geoip_cn
        exec: jump add_to_cn_set
      - matches:
          - has_resp
        exec: return
      - exec: $forward_remote

  # ============================================================================
  # 主流程：依次匹配 → 决定走国内还是国外
  # ============================================================================
  - tag: main_sequence
    type: sequence
    args:
      # 1) 静态 hosts 命中 → 直接返回
      - exec: $user_hosts

      # 2) 缓存命中 → 直接返回
      - exec: $cache

      # 3) 用户白名单 → 走国内
      - matches:
          - qname $user_whitelist
        exec: $forward_local

      - matches:
          - has_resp
        exec: jump add_to_cn_set

      # 4) 用户黑名单 → 走国外（DoH）
      - matches:
          - qname $user_blacklist
        exec: $forward_remote

      - matches:
          - has_resp
        exec: return

      # 5) geosite_cn → 走国内
      - matches:
          - qname $geosite_cn
        exec: $forward_local

      - matches:
          - has_resp
        exec: jump add_to_cn_set

      # 6) geosite_no_cn → 走国外
      - matches:
          - qname $geosite_no_cn
        exec: $forward_remote

      - matches:
          - has_resp
        exec: return

      # 7) 未匹配的域名：本地优先解析，国内 IP 直接采用，否则远程兜底
      - exec: $primary_local_secondary_remote

  # ============================================================================
  # 服务端：监听 53 端口（UDP + TCP）
  # ============================================================================
  - tag: udp_server
    type: udp_server
    args:
      entry: main_sequence
      listen: ":53"

  - tag: tcp_server
    type: tcp_server
    args:
      entry: main_sequence
      listen: ":53"
