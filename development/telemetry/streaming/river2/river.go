package river

var (
  vars      map[string]Var
  monitors atomic.Value // []Monitor
  addCh chan addRequest
  removeCh chan removeRequest
)

type service struct {}

func newService() (*service) {
  s := &service{}
  go s.subLoop()
}

func (s *service) subLoop() {
  for {
    select {
    case a := addCh:
      sl := monitors.Load().([]Monitor)
      sl = append(sl, conn)
    case r := removeCh:

    }
  }
}

func (s *service) Subscribe(ctx context.Context, *pb.SubscribeReq, stream grpc.Stream) error {

}

func (s *service) AddVars(ctx context.Context, *pb.AddVarsReq, *pb.AddVarsResp) error {

}

func (s *service) RemoveVars(ctx context.Context, *pb.RemoveVarsReq, *pb.RemoveVarsResp) error {

}

func (s *service) ChangeInterval(ctx context.Context, *pb.ChangeIntervalReq, *pb.ChangeIntervalResp) error {

}

type Var interface {
    // String returns a valid JSON value for the variable.
    // Types with String methods that do not return valid JSON
    // (such as time.Time) must not be used as a Var.
    String() string

    isVar()
}

// Publish declares a named exported variable. This MUST be called from a
// package's init function when it creates its Vars. If the name is already
// registered then this will log.Panic.
func Publish(name string, v Var, minInterval time.Duration) {
  if _, dup := vars[name]; dup {
		log.Panicln("Reuse of exported var name:", name)
	}
  vars[name] = v
}

// Mux returns the grpc.Muxer for the publisher.  You must start this for
// monitoring to start.
func Mux() {

}

type Int struct {
	i int64
  lastSent time.Time
}

func (v *Int) Value() int64 {
	return atomic.LoadInt64(&v.i)
}

func (v *Int) String() string {
	return strconv.FormatInt(atomic.LoadInt64(&v.i), 10)
}

func (v *Int) Add(delta int64) {
	atomic.AddInt64(&v.i, delta)
}

func (v *Int) Set(value int64) {
	atomic.StoreInt64(&v.i, value)
}

// Map is a string-to-Var map variable that satisfies the Var interface.
  type Map struct {
  	m      sync.Map // map[string]Var
  	keysMu sync.RWMutex
  	keys   []string // sorted
  }

  // KeyValue represents a single entry in a Map.
  type KeyValue struct {
  	Key   string
  	Value Var
  }

  func (v *Map) String() string {
  	var b bytes.Buffer
  	fmt.Fprintf(&b, "{")
  	first := true
  	v.Do(func(kv KeyValue) {
  		if !first {
  			fmt.Fprintf(&b, ", ")
  		}
  		fmt.Fprintf(&b, "%q: %v", kv.Key, kv.Value)
  		first = false
  	})
  	fmt.Fprintf(&b, "}")
  	return b.String()
  }

  func (v *Map) Init() *Map { return v }

  // updateKeys updates the sorted list of keys in v.keys.
  func (v *Map) addKey(key string) {
  	v.keysMu.Lock()
  	defer v.keysMu.Unlock()
  	v.keys = append(v.keys, key)
  	sort.Strings(v.keys)
  }

  func (v *Map) Get(key string) Var {
  	i, _ := v.m.Load(key)
  	av, _ := i.(Var)
  	return av
  }

  func (v *Map) Set(key string, av Var) {
  	// Before we store the value, check to see whether the key is new. Try a Load
  	// before LoadOrStore: LoadOrStore causes the key interface to escape even on
  	// the Load path.
  	if _, ok := v.m.Load(key); !ok {
  		if _, dup := v.m.LoadOrStore(key, av); !dup {
  			v.addKey(key)
  			return
  		}
  	}

  	v.m.Store(key, av)
  }

  // Add adds delta to the *Int value stored under the given map key.
  func (v *Map) Add(key string, delta int64) {
  	i, ok := v.m.Load(key)
  	if !ok {
  		var dup bool
  		i, dup = v.m.LoadOrStore(key, new(Int))
  		if !dup {
  			v.addKey(key)
  		}
  	}

  	// Add to Int; ignore otherwise.
  	if iv, ok := i.(*Int); ok {
  		iv.Add(delta)
  	}
  }

  // AddFloat adds delta to the *Float value stored under the given map key.
  func (v *Map) AddFloat(key string, delta float64) {
  	i, ok := v.m.Load(key)
  	if !ok {
  		var dup bool
  		i, dup = v.m.LoadOrStore(key, new(Float))
  		if !dup {
  			v.addKey(key)
  		}
  	}

  	// Add to Float; ignore otherwise.
  	if iv, ok := i.(*Float); ok {
  		iv.Add(delta)
  	}
  }


func Do(f func(KeyValue))
