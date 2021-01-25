package etcd

import (
	"context"
	"sync"
	"time"

	"github.com/johnsiilver/golib/development/distlock"

	etcd "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

func init() {
	l := newLocker()
	distlock.Register("etcd://", distlock.RegEntry{l, l, l})
}

// Handler provides for locking and unlocking resources.
type Locker struct {
	mu sync.Mutex
	clients map[string]*etcd.Client
}

func newLocker() distlock.Locker {
	return Locker{
		clients: map[string]*etcd.Client{},
	}
}

func (l *Locker) TryLock(ctx context.Context, service, resource string, timeout time.Duration, options ...distlock.LockOption) (distlock.Releaser, error) {
	prefix = "/etcd-lock"

	opts := distlock.LockOptions{}
	for _, o := range options {
		o(&opts)
	}

	if opts.TTL < 5*time.Second {
		opts.TTL = 30 * time.Second
	}

	// Retrieve client.
	// TODO(johnsiilver): Move to method.
	var client *etcd.Client
	var ok bool
	func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if client, ok = client.clients[resource]; !ok {

		}
	}()


	session, err := concurrency.NewSession(client, concurrency.WithTTL(int(timeout/time.Second))
	if err != nil {
		return nil, err
	}
	key := path.Join(prefix, resource)
	mutex := concurrency.NewMutex(session, key)

	if !wait {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	err = mutex.Lock(ctx)
	if err == context.DeadlineExceeded {
		return nil, distlock.LockAcquireTimeoutErr
	}

	etcLock := &EtcdLock{mutex: mutex, Mutex: &sync.Mutex{}}
	releaseCh := make(chan struct{})
	lostCh := make(chan struct{})
	closeOnce := &sync.Once{}

	lock := Locker{
		Lost: lostCh,
		Releaser: func(options ...UnlockOption) error {
			closeOnce.Do(
				func() {
						close(releaseCh)
				}
			)
		}
	}

	go func() {
		for {
			select {
			case <-l.renewed():
				panic("need to add")
			case <-releaseCh:
				etcLock.Release()
				close(lostCh)
			case <-time.After(time.Now().Add(timeout)):
				etcLock.Release()
				close(lostCh)
			}
		}
	}()

	// Release the lock after the end of the TTL automatically
	go func() {
		select {
		case <-time.After(time.Duration(ttl) * time.Second):
			etcLock.Release()
		}
	}()

	return lock, nil
}

func (l *Locker) Release(service, resource string, options ...distlock.UnlockOption) error {

}

func (l *Locker) Info(service, resource string) (Info, error) {

}

func (l *Locker) Read(service, resource string, i interface{}) error {

}
