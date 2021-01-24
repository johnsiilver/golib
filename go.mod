module github.com/johnsiilver/golib

go 1.15

replace (
	github.com/coreos/etcd => ./etcd-fix
	github.com/coreos/etcd/clientv3 => ./etcd-fix/client/v3
	github.com/coreos/etcd/clientv3/concurrency => ./etcd-fix/client/v3/concurrency
	go.etcd.io/etcd => ./etcd-fix
	go.etcd.io/etcd/api/v3 => ./etcd-fix/api
	go.etcd.io/etcd/client/v2 => ./etcd-fix/client/v2
	go.etcd.io/etcd/client/v3 => ./etcd-fix/client/v3
	go.etcd.io/etcd/etcdctl/v3 => ./etcd-fix/etcdctl
	go.etcd.io/etcd/pkg/v3 => ./etcd-fix/pkg
	go.etcd.io/etcd/raft/v3 => ./etcd-fix/raft
	go.etcd.io/etcd/server/v3 => ./etcd-fix/server
	go.etcd.io/etcd/tests/v3 => ./etcd-fix/tests
)

require (
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/beeker1121/goque v2.1.0+incompatible
	github.com/frankban/quicktest v1.11.3 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-ole/go-ole v1.2.5 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.3
	github.com/google/uuid v1.2.0
	github.com/johnsiilver/boutique v0.1.1-beta.1
	github.com/kr/pretty v0.2.1
	github.com/kylelemons/godebug v1.1.0
	github.com/lukechampine/freeze v0.0.0-20160818180733-f514e08ae5a0
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/pborman/uuid v1.2.1
	github.com/pierrec/lz4 v2.6.0+incompatible
	github.com/shirou/gopsutil v3.20.12+incompatible
	github.com/spf13/pflag v1.0.5
	github.com/syndtr/goleveldb v1.0.0 // indirect
	go.etcd.io/etcd/client/v3 v3.0.0-00010101000000-000000000000
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad
	golang.org/x/net v0.0.0-20210119194325-5f4716e94777
	golang.org/x/sys v0.0.0-20210124154548-22da62e12c0c
	google.golang.org/grpc v1.35.0
)
