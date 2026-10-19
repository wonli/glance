# Windows 内存采集

正式 Windows 包和 `cmd/glance` 共用诊断入口。默认关闭；监听地址由 `GLANCE_PPROF_ADDR` 显式指定。
完全退出旧 Glance 进程（包括托盘），再从 PowerShell 启动新构建：

```powershell
$env:GLANCE_PPROF_ADDR = "0.0.0.0:6060"
Start-Process ".\glance-windows-amd64-latest.exe"
```

浏览器打开 `http://127.0.0.1:6060/debug/pprof/`。局域网其他电脑使用 `http://<Windows电脑的局域网IP>:6060/debug/pprof/`，下面采集命令中的 `127.0.0.1` 也替换成该 IP。若无法连接，在 Windows 防火墙中允许该程序在专用网络访问，或添加仅允许本地子网的 TCP 6060 入站规则。pprof 没有身份验证，调试结束后关闭监听；不要把端口映射到公网。

端口占用或地址无效会写入 `%APPDATA%\Glance\startup.log`，应用仍正常启动。
不需要安装 Go 就能下载快照。以下命令在 PowerShell 中执行：

1. 空窗口稳定后采集基线，并记录任务管理器中的进程内存数值及列名。

```powershell
curl.exe -f -o empty.heap "http://127.0.0.1:6060/debug/pprof/heap?gc=1"
```

2. 打开目标 Word，等待分页结束、内存稳定，先记录进程内存并采集自然状态，再采集 GC 后状态；再次记录进程内存。

```powershell
curl.exe -f -o word-before-gc.heap "http://127.0.0.1:6060/debug/pprof/heap"
curl.exe -f -o word.heap "http://127.0.0.1:6060/debug/pprof/heap?gc=1"
```

3. 关闭文档后再采集，并记录进程内存。

```powershell
curl.exe -f -o closed.heap "http://127.0.0.1:6060/debug/pprof/heap?gc=1"
```

`gc=1` 会主动触发 GC，可能短暂影响交互，但不保证把空闲内存立即归还操作系统。
默认二进制 heap 是用于分析的文件，不是浏览器中的可视化图表。安装 Go 的机器可运行：

```powershell
go tool pprof -top -inuse_space word.heap
go tool pprof -top -inuse_space -base empty.heap word.heap
go tool pprof -http=127.0.0.1:8080 word.heap
```

交互图形视图可能需要 Graphviz；`-top` 不需要。没有 Go 时也可下载文本概要：

```powershell
curl.exe -f -o word-heap.txt "http://127.0.0.1:6060/debug/pprof/heap?debug=1"
```

优先分析 `inuse_space`（存活对象），而不是把累计分配量当常驻占用。
heap 仅覆盖 Go 堆，不能直接解释任务管理器中的全部进程占用，尤其是 GPU 和原生分配。
保留同次构建的可执行文件以供符号分析。诊断结束后退出应用、移除变量并重启：

```powershell
Remove-Item Env:GLANCE_PPROF_ADDR
```
