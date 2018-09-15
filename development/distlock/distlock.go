// Package distlock provides access to distributed locking mechanisms for the
// purpose of locking resources across a distributed system.
// Locker provides the standard interface for locking.  To use a specific lock
// service, you must import a locking implementation from a subdirectory.
// Note: If faking the interfaces here, you MUST embed the interface in your
// fakes. Otherwise future changes will break your code.
package distlock

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	LockAcquireTimeoutErr = fmt.Errorf("Timeout trying to acquire lock")
)

// LockOptions provides options to various Locker implementations.
// This is not set directly but constructed by using implementations of LockOption.
type LockOptions struct {
	// TTL is the minimum time to hold the lock. This can be no less than 5 seconds.
	// TTL only counts as a minimum time in case lock renewal fails.  A lock
	// can expire imediately by calling Release() or Unlocker().
	// TTLs have upper limits that can prevent the TTL being set from being implemented.
	TTL time.Duration
}

// LockOption provides an optional argument for locking a resource.
type LockOption func(o *LockOptions)

// UnlockOptions provides options to various Locker implementations.
// This is not set directly but constructed by using implementations of UnlockOption.
type UnlockOptions struct{}

// UnlockOption provides an optional argument for unlocking a resource.
type UnlockOption func(o *LockOptions)

// TTL sets the minimum time the lock will be held.
// If set below 5 seconds, will panic.
func TTL(d time.Duration) LockOption {
	return func(o *LockOptions) {
		if d < 5*time.Second {
			panic("cannot set a TTL for less than 5 seconds")
		}
		o.TTL = d
	}
}

// Locker provides access to all functions for interfacing with locks.
// Not all Locker methods may be provided by all implementations.  Only
// Lock()/Unlock() are guaranteed.
type Locker interface {
	Handler
	Infoer
	Reader
	Watcher
}

// Handler provides for locking and unlocking resources.
type Handler interface {
	// Lock locks a resource at service location. Implementations will receive
	// the service without the namespace (whatever://).
	TryLock(ctx context.Context, service, resource string, options ...LockOption) (Lock, error)
	// Unlock unlocks a resource at service location.  Implemntations will receive
	// the service without the namespace (whatever://).
	Release(ctx context.Context, service, resource string, options ...UnlockOption) error
}

// Infoer allows getting information about a lock.
type Infoer interface {
	// Info returns information about a Lock.
	Info(ctx context.Context, service, resource string) (Info, error)
}

// Reader allows reading data held in a lock, if the lock can hold data.
type Reader interface {
	// Read reads the data in the lock into i.
	Read(ctx context.Context, service, resource string) ([]byte, error)
}

// Watcher watches the content held in a lock and returns the content
// whenever it changes. Watch will return immediately any content in the lock.
type Watcher interface {
	Watch(ctx context.Context, service, resource string) (chan []byte, Cancel, error)
}

// Lock provides information and operations for a lock that has been taken out.
type Lock struct {
	// Lost is closed when the lock is lost.
	Lost chan struct{}

	// Release can be called to release a lock.
	Release Releaser
}

// Releaser unlocks a resource.
type Releaser func(options ...UnlockOption) error

// Cancel cancels a background function.
type Cancel func()

// Info provides information about a lock.
type Info struct {
	// LockTime is the time the lock was taken out.
	LockTime time.Time
	// ExpireTime is the time the lock expires if not updated.
	ExpireTime time.Time
	// Owner is the identitiy of who owns the lock.  This is lock type specific.
	Owner interface{}
}

// IsZero indicates if Info is the zero value.
func (i Info) IsZero() bool {
	switch {
	case !i.LockTime.IsZero():
		return false
	case !i.ExpireTime.IsZero():
		return false
	case i.Owner != nil:
		return false
	}
	return true
}

// New constructs a new Locker. The Locker returned can access any
// Locker registered with this module.
func New() (Locker, error) {
	return &lockerMux{}, nil
}

type lockerMux struct{}

// TryLock implements Locker.TryLock().
func (l *lockerMux) TryLock(ctx context.Context, service, resource string, options ...LockOption) (Lock, error) {
	location, entry, err := l.split(service)
	if err != nil {
		return Lock{}, err
	}

	return entry.Locker.TryLock(ctx, location, resource, options...)
}

// Release implements Locker.Unlock().
func (l *lockerMux) Release(ctx context.Context, service, resource string, options ...UnlockOption) error {
	location, entry, err := l.split(service)
	if err != nil {
		return err
	}

	return entry.Locker.Release(ctx, location, resource, options...)
}

func (l *lockerMux) Info(ctx context.Context, service, resource string) (Info, error) {
	location, entry, err := l.split(service)
	if err != nil {
		return Info{}, err
	}

	if entry.Infoer == nil {
		return Info{}, fmt.Errorf("lock service does not provide Info()")
	}

	return entry.Infoer.Info(ctx, location, resource)
}

// Read implements Reader.Read().
func (l *lockerMux) Read(ctx context.Context, service, resource string) ([]byte, error) {
	location, entry, err := l.split(service)
	if err != nil {
		return nil, err
	}

	if entry.Reader == nil {
		return nil, fmt.Errorf("lock service does not provide Read()")
	}

	return entry.Reader.Read(ctx, location, resource)
}

func (l *lockerMux) Watch(ctx context.Context, service, resource string) (chan []byte, Cancel, error) {
	location, entry, err := l.split(service)
	if err != nil {
		return nil, nil, err
	}

	if entry.Watcher == nil {
		return nil, nil, fmt.Errorf("lock service does not provide Watch()")
	}
	return entry.Watcher.Watch(ctx, location, resource)
}

func (l *lockerMux) split(service string) (location string, entry RegEntry, err error) {
	sp := strings.SplitAfter("://", service)
	if len(sp) != 2 {
		return "", RegEntry{}, fmt.Errorf("service %s does not correctly define a namespace", sp)
	}
	if entry, ok := registry[sp[0]]; ok {
		return sp[1], entry, nil
	}
	return "", RegEntry{}, fmt.Errorf("could not find Locker for namespace %s", sp[0])
}

// RegEntry describes a Locker entry in the internal registry.
type RegEntry struct {
	Locker  Locker
	Infoer  Infoer
	Reader  Reader
	Watcher Watcher
}

var registry = map[string]RegEntry{}

var nameRE = regexp.MustCompile(`^[a-z_]{1,}://$`)

// Register registers a Locker for a namespace.  A namespace must be:
// [a-z_]*://, like etcd:// .
func Register(namespace string, entry RegEntry) error {
	if !nameRE.MatchString(namespace) {
		return fmt.Errorf("could not register namespace %q, not a valid name", namespace)
	}
	if _, ok := registry[namespace]; ok {
		return fmt.Errorf("%s was already a registered namespace for distlock.Locker", namespace)
	}
	if entry.Locker == nil {
		return fmt.Errorf("Locker cannot be nil when calling Register for namespace %s", namespace)
	}

	registry[namespace] = entry
	return nil
}
