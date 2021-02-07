package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"math"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
)

var (
	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
)

var statsTemplText = `
Test Results({{.RPCType}}):
==========================================================================
[Speed]
{{ range .Stats }}
[{{.Concurrency}} Users][{{.Count}} Requests][{{.Bytes}} Bytes] - min {{.MinRTT}}/sec, max {{.MaxRTT}}/sec, avg {{.AvgRTT}}/sec, rps {{.RPS}}
{{ end }}

[Allocs]
{{ range .Allocs }}
[{{.Count}} Requests][{{.Bytes}} Bytes] - allocs {{.AllocsStr}}
{{ end }}
`
var statsTempl = template.Must(template.New("stats").Parse(statsTemplText))

const (
	GRPC = "grpc"
	UDS  = "uds"
)

type testStats struct {
	RPCType string
	Stats   []*Stats
	Allocs  []Allocs
}

type Allocs struct {
	Concurrency int
	Count       int
	DataSize    int
	Allocs      float64
}

func (a Allocs) Bytes() string {
	return humanize.Bytes(uint64(a.DataSize))
}

func (a Allocs) AllocsStr() string {
	return humanize.Comma(int64(a.Allocs))
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

type testParms struct {
	Concurrency int
	Amount      int
	PacketSize  int
	Alloc       bool
}

func main() {
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	bench := []struct {
		RPCType string
		Tests   []testParms
	}{
		{
			RPCType: UDS,
			Tests: []testParms{
				{runtime.NumCPU(), 10000, 1024, false},
				{runtime.NumCPU(), 10000, 1024 * 10, false},
				{runtime.NumCPU(), 10000, 1024 * 100, false},
				{runtime.NumCPU(), 10000, 1024 * 1000, false},
				{1, 10000, 1024, true},
				{1, 10000, 1024 * 10, true},
				{1, 10000, 1024 * 100, true},
				{1, 10000, 1024 * 1000, true},
			},
		},
		{
			RPCType: GRPC,
			Tests: []testParms{
				{runtime.NumCPU(), 10000, 1024, false},
				{runtime.NumCPU(), 10000, 1024 * 10, false},
				{runtime.NumCPU(), 10000, 1024 * 100, false},
				{runtime.NumCPU(), 10000, 1024 * 1000, false},
				{runtime.NumCPU(), 10000, 1024, true},
				{1, 10000, 1024 * 10, true},
				{1, 10000, 1024 * 100, true},
				{1, 10000, 1024 * 1000, true},
			},
		},
	}

	for _, b := range bench {
		log.Println("Running tests for: ", b.RPCType)
		var stats = testStats{RPCType: b.RPCType}
		for _, test := range b.Tests {
			log.Printf("\tTest Params: %#v", test)
			if test.Alloc {
				switch b.RPCType {
				case UDS:
					a := testing.AllocsPerRun(
						2,
						func() {
							udsTest(test.Concurrency, test.Amount, test.PacketSize)
						},
					)
					stats.Allocs = append(stats.Allocs, Allocs{test.Concurrency, test.Amount, test.PacketSize, a})
				case GRPC:
					a := testing.AllocsPerRun(
						2,
						func() {
							grpcTest(test.Concurrency, test.Amount, test.PacketSize)
						},
					)
					stats.Allocs = append(stats.Allocs, Allocs{test.Concurrency, test.Amount, test.PacketSize, a})
				default:
					panic("unsupported RPCType")
				}
			} else {
				switch b.RPCType {
				case UDS:
					stats.Stats = append(stats.Stats, udsTest(test.Concurrency, test.Amount, test.PacketSize))
				case GRPC:
					stats.Stats = append(stats.Stats, grpcTest(test.Concurrency, test.Amount, test.PacketSize))
				default:
					panic("unsupported RPCType")
				}
			}
		}

		if err := statsTempl.Execute(os.Stdout, stats); err != nil {
			panic(err)
		}
		time.Sleep(1 * time.Second)
	}
}

type records struct {
	sync.Mutex
	benchmarks []benchmarks
}

func (r *records) put(b benchmarks) {
	r.Lock()
	r.benchmarks = append(r.benchmarks, b)
	r.Unlock()
}

type Stats struct {
	StartTime, EndTime    time.Time
	Concurrency, DataSize int

	Count                    int64
	MinRTT, MaxRTT, TotalRTT time.Duration
}

func newStats(concurrency, dataSize int, r *records, start, end time.Time) *Stats {
	s := &Stats{
		StartTime:   start,
		EndTime:     end,
		Concurrency: concurrency,
		DataSize:    dataSize,
		MinRTT:      time.Duration(math.MaxInt64),
	}

	for _, b := range r.benchmarks {
		s.addStats(b)
	}
	return s
}

func (s *Stats) addStats(b benchmarks) {
	s.Count++

	rtt := b.endTime.Sub(b.startTime)
	s.TotalRTT += rtt
	if rtt < s.MinRTT {
		s.MinRTT = rtt
	}
	if rtt > s.MaxRTT {
		s.MaxRTT = rtt
	}
}

func (s *Stats) Bytes() string {
	return humanize.Bytes(uint64(s.DataSize))
}

func (s *Stats) AvgRTT() time.Duration {
	return s.TotalRTT / time.Duration(s.Count)
}

func (s *Stats) RPS() string {
	runtimeSecs := float64(s.EndTime.Sub(s.StartTime)) / float64(time.Second)
	rps := float64(s.Count) / runtimeSecs
	return fmt.Sprintf("%.2f", rps)
}

type benchmarks struct {
	startTime time.Time
	endTime   time.Time
}

type call func() benchmarks

type callPool struct {
	in      chan call
	records *records
	wg      sync.WaitGroup
}

func newPool(size int) *callPool {
	cp := &callPool{
		in:      make(chan call, 1),
		records: &records{},
	}
	cp.wg.Add(size)
	for i := 0; i < size; i++ {
		go cp.caller()
	}
	return cp
}

func (c *callPool) caller() {
	defer c.wg.Done()

	for call := range c.in {
		c.records.put(call())
	}
}
