# UDS - Packages for using Unix Domain Sockets

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

gRPC works great for IPC on UDS, at least with protos. While I love gRPC, I wanted more options and I wanted authentication. 

## Everyone loves some benchmarks

So I certainly haven't benched everything. But I did bench the proto RPC, as it uses the chunking rpc package, which uses chunked streams. 

I compared this to gRPC. Interestingly, they seem to have small and large requests on me, while I seem to hold the middle ground. At different points we both double the other's performance. 

At some point I will dig more into this, see if I can beat them at all of them.

Platform was OSX running on an 8-core Macbook Pro, Circa 2019. You can guess that your Threadripper Linux box will do much better.

```
Test Results(uds):
==========================================================================

[16 Users][10000 Requests][1.0 kB Bytes] - min 105.798µs/sec, max 10.012826ms/sec, avg 345.763µs/sec, rps 46036.38

[16 Users][10000 Requests][10 kB Bytes] - min 183.494µs/sec, max 7.120556ms/sec, avg 782.938µs/sec, rps 20370.44

[16 Users][10000 Requests][102 kB Bytes] - min 754.436µs/sec, max 13.988786ms/sec, avg 2.397784ms/sec, rps 6662.04

[16 Users][10000 Requests][1.0 MB Bytes] - min 8.938606ms/sec, max 101.601032ms/sec, avg 77.746482ms/sec, rps 205.74

Test Results(grpc):
==========================================================================

[16 Users][10000 Requests][1.0 kB Bytes] - min 53.877µs/sec, max 1.971816ms/sec, avg 300.443µs/sec, rps 51995.80

[16 Users][10000 Requests][10 kB Bytes] - min 95.035µs/sec, max 3.520594ms/sec, avg 898.786µs/sec, rps 17747.95

[16 Users][10000 Requests][102 kB Bytes] - min 2.363614ms/sec, max 8.20306ms/sec, avg 4.60433ms/sec, rps 3473.11

[16 Users][10000 Requests][1.0 MB Bytes] - min 21.394619ms/sec, max 40.292218ms/sec, avg 34.239825ms/sec, rps 467.15
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