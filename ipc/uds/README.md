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

I compared this to gRPC. Interestingly, they seem to handle the smallest data requests (1.0 kB) than this package, but this seems to be do better for all others. I think think in the small data size it is because I have a lot of memory pools for reuse and slightly more overhead in my packets.

No matter what my internal settings, I would beat gRPC at 10kB and double their performance in the 102 kB size.  To get better performance in large sizes, I had to add some kernel buffer space over the defaults, which lead to close to double performance.

Platform was OSX running on an 8-core Macbook Pro, Circa 2019. You can guess that your Threadripper Linux box will do much better.

```
Test Results(uds):
==========================================================================

[16 Users][10000 Requests][1.0 kB Bytes] - min 66.006µs/sec, max 10.490104ms/sec, avg 360.717µs/sec, rps 44073.80

[16 Users][10000 Requests][10 kB Bytes] - min 316.524µs/sec, max 7.427324ms/sec, avg 695.233µs/sec, rps 22937.87

[16 Users][10000 Requests][102 kB Bytes] - min 1.67625ms/sec, max 17.691309ms/sec, avg 2.593304ms/sec, rps 6159.70

[16 Users][10000 Requests][1.0 MB Bytes] - min 9.369687ms/sec, max 57.597223ms/sec, avg 19.979866ms/sec, rps 800.25

Test Results(grpc):
==========================================================================

[16 Users][10000 Requests][1.0 kB Bytes] - min 55.612µs/sec, max 2.611833ms/sec, avg 318.452µs/sec, rps 49159.45

[16 Users][10000 Requests][10 kB Bytes] - min 92.618µs/sec, max 3.379955ms/sec, avg 914.139µs/sec, rps 17444.59

[16 Users][10000 Requests][102 kB Bytes] - min 1.932897ms/sec, max 14.351104ms/sec, avg 4.529876ms/sec, rps 3530.03

[16 Users][10000 Requests][1.0 MB Bytes] - min 15.468868ms/sec, max 39.040674ms/sec, avg 31.875585ms/sec, rps 501.78
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