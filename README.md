# tcp-zstd-proxy

**tcp-zstd-proxy** is a lightweight TCP proxy that uses zstd compression to optimize data transmission. It transparently compresses and decompresses TCP traffic, which can reduce bandwidth usage and boost performance in high-speed data transfer scenarios.

## Installation

Clone the repository and build the binary using Go:

```bash
git clone https://github.com/yourusername/tcp-zstd-proxy.git
cd tcp-zstd-proxy
go build -o tcp-zstd-proxy .
```
or download the binary from the release page

## Usage

```bash
tcp-zstd-proxy -target localhost:5201 -listen :8888 -listen-compress #server side
tcp-zstd-proxy -target localhost:8888 -listen :5205 -remote-compress #client side
```

## Explain
:5201 is iperf3 tcp port
:8888 is compressed port
:5205 is uncompressed port 

The traffic flow is as follows:
```
:5201 <-tcp-> :8888 <-zstd tcp-> :5205
```

## Performance

```bash
iperf3 -s #server side 
iperf3 -c localhost -p 5205 #client side
```

### performance results:
5GB/s iperf3 on a 12th gen intel cpu