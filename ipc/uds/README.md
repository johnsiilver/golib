# UDS - Packages for using Unix Domain Sockets

## Usage Note
This package is considered beta. Until this note is removed, changes to this that cause an incompatibility will be noted with a Minor version change, not a Major version change in
the overall repo.

## TLDR

- Provides convience over standard net.Listen("unix") and net.Dial("unix")
- Provides high level packages to do streams and RPCs(protocol buffers/JSON)
- Provides authentication on the server
- Benchmarked against gRPC, mostly faster(except at 1kiB or lower) and WAY LESS allocations
- Linux/OSX only

## Introduction
Go provides unix sockets via its net.Dial() and net.Listen() calls.  But how to use these to achive IPC (interprocess communication) isn't straightforward.

This package and its sub-packages provide authenticated streaming and RPCs utilizing:
- Data chunks (whatever you want to send in []byte)
- JSON messages
- Protocol buffer(proto) messages

If you simply want to use UDS with an io.ReadWriteCloser interface without
dealing with all the Dial()/Listen() noise (plus you get authentication), the main uds/ package is for you.

Works on Linux and OSX only. Haven't investigated all the BSD syscall options needed and don't want to tackle Windows (but I do take PRs).

## Use cases

You need to do IPC for something cross platform (or at least Linux and OSX) and
want that communication authenticated.

UDS is on most platforms (though not all options are). However, it is not the fastest IPC. 

Shared memory seems to be the fasted (but its a pain). Various message queuing destroys UDS in performance, but they aren't very cross platform (so you'd have to use a different one per platform).

I've used UDS to communicate to sub-processes when I want to mount and unmount "plugins".  Sometimes between languages.

## Alternatives

There are some other packages out there, but I haven't seen them supporting auth.

gRPC works great for IPC on UDS, at least with protos. While I love gRPC, I wanted more options, authentication and much lower heap allocations.

## Everyone loves some benchmarks

So I certainly haven't benched everything. But I did bench the proto RPC, as it uses the chunking rpc package, which uses chunked streams. 

I compared this to gRPC as it provides a managed RPC mechansim over unix sockets. Interestingly, gRPC seem to handle the smallest data requests (1.0 kB) better than this package, but this seems to be do better for all others. I think the small data size test is due to my use of memory pools for reuse and slightly more overhead in my packets.

No matter what my internal settings, I would beat gRPC at 10kB and double their performance in the 102 kB size.  To get better performance in large sizes, I had to add some kernel buffer space over the defaults, which lead to close to double performance.

But the real killer here is allocations. This decimates gRPC in heap allocation reduction. Keep this in mind for high performance applications. If you delve deep into making Go fast, everything points at one thing: keeping your allocations down. Once you leave the micro-benchmark world (like these benchmarks), your app starts crawling if the GC has to deal with lots of objects. The key to that is buffer reuse and gRPC's design for ease of use hurts that ability.

Platform was OSX running on an 8-core Macbook Pro, Circa 2019. You can guess that your Threadripper Linux box will do much better.

```
Test Results(uds):
==========================================================================
[Speed]

[16 Users][10000 Requests][1.0 kB Bytes] - min 65.286µs/sec, max 5.830322ms/sec, avg 373.437µs/sec, rps 42534.71

[16 Users][10000 Requests][10 kB Bytes] - min 108.214µs/sec, max 7.875121ms/sec, avg 576.661µs/sec, rps 27596.17

[16 Users][10000 Requests][102 kB Bytes] - min 760.896µs/sec, max 13.829294ms/sec, avg 1.188321ms/sec, rps 13419.44

[16 Users][10000 Requests][1.0 MB Bytes] - min 7.26139ms/sec, max 80.21521ms/sec, avg 14.509374ms/sec, rps 1101.91


[Allocs]

[10000 Requests][1.0 kB Bytes] - allocs 290,365

[10000 Requests][10 kB Bytes] - allocs 300,598

[10000 Requests][102 kB Bytes] - allocs 302,142

[10000 Requests][1.0 MB Bytes] - allocs 309,110

Test Results(grpc):
==========================================================================
[Speed]

[16 Users][10000 Requests][1.0 kB Bytes] - min 59.624µs/sec, max 3.571806ms/sec, avg 305.171µs/sec, rps 51137.15

[16 Users][10000 Requests][10 kB Bytes] - min 93.19µs/sec, max 2.397846ms/sec, avg 875.864µs/sec, rps 18216.72

[16 Users][10000 Requests][102 kB Bytes] - min 1.221068ms/sec, max 8.495421ms/sec, avg 4.434272ms/sec, rps 3606.63

[16 Users][10000 Requests][1.0 MB Bytes] - min 21.448849ms/sec, max 54.920306ms/sec, avg 34.307985ms/sec, rps 466.28


[Allocs]

[10000 Requests][1.0 kB Bytes] - allocs 1,505,165

[10000 Requests][10 kB Bytes] - allocs 1,681,061

[10000 Requests][102 kB Bytes] - allocs 1,947,529

[10000 Requests][1.0 MB Bytes] - allocs 3,250,309
```

Benchmark Guide: 
```
[# Users] = number of simultaneous clients
[# Requests] = the total number of requests we sent
[# Bytes] = the size of our input and output requests
min = the minimum seen RTT
max = the maximum seen RTT
av = the average seen RTT
rps = requests per second
```
