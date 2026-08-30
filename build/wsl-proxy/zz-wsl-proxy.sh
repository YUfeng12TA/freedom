# /etc/profile.d/zz-wsl-proxy.sh —— WSL2 镜像网络下的智能代理（宿主 v2rayN 127.0.0.1:10808）
# 策略：登录 shell 时探测代理端口，活着才自动启用（代理关掉时 WSL 网络不受拖累）；
#      另提供 proxy_on / proxy_off 手动开关。静默运行，不污染终端。

__wsl_proxy_host="127.0.0.1"
__wsl_proxy_port="10808"

__wsl_proxy_alive() {
    timeout 1 bash -c "echo > /dev/tcp/$__wsl_proxy_host/$__wsl_proxy_port" 2>/dev/null
}

proxy_on() {
    local p="http://$__wsl_proxy_host:$__wsl_proxy_port"
    export http_proxy="$p" https_proxy="$p" all_proxy="socks5://$__wsl_proxy_host:$__wsl_proxy_port"
    export no_proxy="localhost,127.0.0.1,::1,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,.local"
    export HTTP_PROXY="$p" HTTPS_PROXY="$p" ALL_PROXY="socks5://$__wsl_proxy_host:$__wsl_proxy_port"
    export NO_PROXY="$no_proxy"
    echo "[proxy] on -> $__wsl_proxy_host:$__wsl_proxy_port"
}

proxy_off() {
    unset http_proxy https_proxy all_proxy no_proxy HTTP_PROXY HTTPS_PROXY ALL_PROXY NO_PROXY
    echo "[proxy] off"
}

proxy_status() {
    if [ -n "$http_proxy" ]; then
        echo "[proxy] active: $http_proxy"
    else
        echo "[proxy] not set (proxy_on / proxy_off 可手动切换)"
    fi
}

# 登录时探测：端口活着自动启用（静默）
if __wsl_proxy_alive; then
    proxy_on >/dev/null
fi
unset -f __wsl_proxy_alive
