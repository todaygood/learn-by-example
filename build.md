# build ebpf program 

安装llvm工具，然后运行go generatate, go build 即可生成二进制程序。

```bash
root@ubunt2:~# cd learn-by-example/ebpf/
acl/                              fentry_fexit-kprobe/              inject/                           tcpoptions/
bpf2bpf/                          fentry_fexit-sockops/             iter/                             tcx/
bpffs/                            fentry_fexit-tailcall/            kfunc_ffs/                        timer/
bpflbr/                           fentry_fexit-tailcall_in_bpf2bpf/ metadata_xdp2afxdp/               toa/
bpfprogfuncs/                     fentry_fexit-tc/                  percpu_data/                      tp_btf/
bsearch/                          fentry_fexit-tracepoint/          spinlock/                         tracepoint/
fd-socket/                        fentry_fexit-xdp/                 tailcall/                         xdp-cpumap/
fentry-bpf2bpf/                   fexit_ipv4_sysctl/                tailcall-in-bpf2bpf/              xdp-crc/
fentry-leak/                      fexit_rpsxps/                     tailcall-in-freplace/             xdp-reset-tcp-conn/
fentry_fexit/                     freplace/                         tailcall-in-freplace1/            xdp-tailcall/
fentry_fexit-bpf2bpf/             global-percpu-data/               tailcall-inspect/                 xdp-traceroute/
fentry_fexit-freplace/            global-variable/                  tailcall-shared/                  xdpmetadata/
fentry_fexit-freplace-xdp/        headers/                          tailcall-stackoverflow/           xdpping/
root@ubunt2:~# cd learn-by-example/ebpf/xdp
xdp-cpumap/         xdp-crc/            xdp-reset-tcp-conn/ xdp-tailcall/       xdp-traceroute/     xdpmetadata/        xdpping/            
root@ubunt2:~# cd learn-by-example/ebpf/xdp-reset-tcp-conn/
root@ubunt2:~/learn-by-example/ebpf/xdp-reset-tcp-conn# ls
a  go.mod  go.sum  main.go  tcp.c
root@ubunt2:~/learn-by-example/ebpf/xdp-reset-tcp-conn# export PATH=$PATH:/opt/LLVM-20.1.0-Linux-X64/bin/
root@ubunt2:~/learn-by-example/ebpf/xdp-reset-tcp-conn# go generate 
Compiled /root/learn-by-example/ebpf/xdp-reset-tcp-conn/tcp_bpfel.o
Stripped /root/learn-by-example/ebpf/xdp-reset-tcp-conn/tcp_bpfel.o
Wrote /root/learn-by-example/ebpf/xdp-reset-tcp-conn/tcp_bpfel.go
Compiled /root/learn-by-example/ebpf/xdp-reset-tcp-conn/tcp_bpfeb.o
Stripped /root/learn-by-example/ebpf/xdp-reset-tcp-conn/tcp_bpfeb.o
Wrote /root/learn-by-example/ebpf/xdp-reset-tcp-conn/tcp_bpfeb.go
root@ubunt2:~/learn-by-example/ebpf/xdp-reset-tcp-conn# ls
a  go.mod  go.sum  main.go  tcp.c  tcp_bpfeb.go  tcp_bpfeb.o  tcp_bpfel.go  tcp_bpfel.o
root@ubunt2:~/learn-by-example/ebpf/xdp-reset-tcp-conn# go build 
root@ubunt2:~/learn-by-example/ebpf/xdp-reset-tcp-conn# ls
a  go.mod  go.sum  main.go  tcp.c  tcp_bpfeb.go  tcp_bpfeb.o  tcp_bpfel.go  tcp_bpfel.o  xdp-reset-tcp-conn
root@ubunt2:~/learn-by-example/ebpf/xdp-reset-tcp-conn# ./xdp-reset-tcp-conn 
2025/04/05 15:03:10 Attached xdp to lo
^Croot@ubunt2:~/learn-by-example/ebpf/xdp-reset-tcp-conn# ./xdp-reset-tcp-conn --help 
Usage of ./xdp-reset-tcp-conn:
  -d, --device string   device to attach XDP program (default "lo")
  -D, --dport uint16    destination port to reset tcp connection (default 65535)
pflag: help requested
root@ubunt2:~/learn-by-example/ebpf/xdp-reset-tcp-conn# 
```


## 测试

```bash
root@ubunt2:~/learn-by-example/ebpf/xdp-reset-tcp-conn# ip r
default via 192.168.59.2 dev ens33 proto dhcp src 192.168.59.131 metric 100 
172.17.0.0/16 dev docker0 proto kernel scope link src 172.17.0.1 linkdown 
192.168.38.0/24 dev ens37 proto kernel scope link src 192.168.38.157 
192.168.59.0/24 dev ens33 proto kernel scope link src 192.168.59.131 metric 100 
192.168.59.2 dev ens33 proto dhcp scope link src 192.168.59.131 metric 100 
192.168.122.0/24 dev virbr0 proto kernel scope link src 192.168.122.1 linkdown 
root@ubunt2:~/learn-by-example/ebpf/xdp-reset-tcp-conn# ./xdp-reset-tcp-conn  -d ens37 -D 22 
2025/04/05 15:11:11 Attached xdp to ens37


hujun469@Taishiji MINGW64 /g
$ ssh root@192.168.38.157
The authenticity of host '192.168.38.157 (192.168.38.157)' can't be established.
ED25519 key fingerprint is SHA256:FPOvBBS1E5f5vMvvmZk8g1pK+Fp6DlXPzsv8JNJ1QsA.
This host key is known by the following other names/addresses:
    ~/.ssh/known_hosts:16: 192.168.31.157
    ~/.ssh/known_hosts:19: 192.168.59.131
Are you sure you want to continue connecting (yes/no/[fingerprint])? yes
Warning: Permanently added '192.168.38.157' (ED25519) to the list of known hosts.
Welcome to Ubuntu 22.04.5 LTS (GNU/Linux 5.15.0-125-generic x86_64)

 * Documentation:  https://help.ubuntu.com
 * Management:     https://landscape.canonical.com
 * Support:        https://ubuntu.com/pro

This system has been minimized by removing packages and content that are
not required on a system that users do not log into.

To restore this content, you can run the 'unminimize' command.
New release '24.04.2 LTS' available.
Run 'do-release-upgrade' to upgrade to it.

Last login: Sat Apr  5 13:56:18 2025 from 192.168.38.1
root@ubunt2:~# client_loop: send disconnect: Connection reset by peer

hujun469@Taishiji MINGW64 /g
$
```