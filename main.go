package main

import (
	"bytes"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/larytet-go/accumulator"
	goutils "gitlab-il.cyren.io/ccs/go-utils"
	"golang.org/x/sync/syncmap"
)

type restAPI struct {
	params        systemParams
	bigMap        *syncmap.Map
	rateQuery     *accumulator.Accumulator
	rateStats     *accumulator.Accumulator
	rateTightLoop *accumulator.Accumulator
	latencyQuery  *accumulator.Accumulator
	latencyStats  *accumulator.Accumulator
	statistics    struct {
		timer100ms uint64
		tick1s     uint64
		sleep10ms  uint64
	}
}

func (ra *restAPI) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	timestamp := time.Now()
	urlPath := strings.ToLower(request.URL.Path[1:])
	switch urlPath {
	case "query":
		ra.rateQuery.Add(1)
		key := request.URL.Query().Get("key")
		if key != "" {
			if valueIfc, ok := ra.bigMap.Load(key); ok {
				fmt.Fprintf(response, "%v\n", valueIfc)
			} else {
				fmt.Fprintf(response, "%v is not found\n", key)
			}
		}
		latency := time.Since(timestamp)
		ra.latencyQuery.Add(uint64(latency))

	case "sample":
		count := 1
		countParam := request.URL.Query().Get("count")
		if countParam != "" {
			count, _ = strconv.Atoi(countParam)
		}
		ra.bigMap.Range(func(key, value interface{}) bool {
			fmt.Fprintf(response, "%v\n", key)
			count--
			if count > 0 {
				return true
			}
			return false
		})

		// Try	while [ 1 ];do echo -en "\\033[0;0H";curl http://127.0.0.1:8081/stat;sleep 0.3;done;
	case "statistics", "", "stat":
		ra.rateStats.Add(1)
		fmt.Fprintf(response, goutils.SprintfStructure(ra.statistics, 5, "%-20s %14v ", []string{}))
		fmt.Fprintf(response, "\n")
		fmt.Fprintf(response, ra.rateQuery.Sprintf("%-28s (requests/s):\n%v\n", "%-28sNo requests in the last %d seconds\n", "%8d ", 16, 1, false))
		fmt.Fprintf(response, "\n")
		fmt.Fprintf(response, ra.rateStats.Sprintf("%-28s (requests/s):\n%v\n", "%-28sNo requests in the last %d seconds\n", "%8d ", 16, 1, false))
		fmt.Fprintf(response, "\n")
		fmt.Fprintf(response, ra.rateTightLoop.Sprintf("%-28s (iterations/s):\n%v\n", "%-28sNo data in the last %d seconds\n", "%8d ", 16, 1, false))
		fmt.Fprintf(response, "\n")
		fmt.Fprintf(response, ra.latencyQuery.Sprintf("%-28s (microseconds):\n%v\n", "%-28sNo requests in the last %d seconds\n", "%8d ", 16, uint64(time.Microsecond), true))
		fmt.Fprintf(response, "\n")
		fmt.Fprintf(response, ra.latencyStats.Sprintf("%-28s (microseconds):\n%v\n", "%-28sNo requests in the last %d seconds\n", "%8d ", 16, uint64(time.Microsecond), true))
		fmt.Fprintf(response, "\n")
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		fmt.Fprintf(response, "Alloc = %v MiB", bToMb(memStats.Alloc))
		latency := time.Since(timestamp)
		ra.latencyStats.Add(uint64(latency))
	}
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

type systemParams struct {
	listenAddress string
	bigMapSize    int
}

type arrayFlags []string

func getParams() (systemParams, error) {
	flag.Set("logtostderr", "true")
	flag.Set("stderrthreshold", "INFO")

	listenAddress := flag.String("listenAddress", ":8081", "HTTP interface")
	bigMapSize := flag.Int("bigMapSize", 10*1000*1000, "Size of the map")

	return systemParams{
		listenAddress: *listenAddress,
		bigMapSize:    *bigMapSize,
	}, nil
}

func populateMap(bigMap *syncmap.Map, count int) {
	bigMap.Store("magic", "key")
	for i := 0; i < count; i++ {
		// I could do faster with []byte instead of String
		key := strconv.FormatUint(rand.Uint64(), 16)
		// Force the compiler to allocate data
		value := bytes.NewBufferString(key)
		bigMap.Store(key, value.Bytes())
	}
}

func main() {

	params, err := getParams()
	if err != nil {
		glog.Errorf("Failed to parse command line arguments %v", err)
		return
	}
	ra := restAPI{
		params:        params,
		bigMap:        &syncmap.Map{},
		rateQuery:     accumulator.New("rateQuery", 60),
		rateStats:     accumulator.New("rateStats", 60),
		rateTightLoop: accumulator.New("rateTightLoop", 60),
		latencyQuery:  accumulator.New("latencyQuery", 60),
		latencyStats:  accumulator.New("latencyStats", 60),
	}

	go func() {
		glog.Infof("Populating map %d entries", params.bigMapSize)
		populateMap(ra.bigMap, params.bigMapSize)
		glog.Infof("Map populated")
		for {
			ra.rateTightLoop.Add(1)
			time.Sleep(1 * time.Microsecond)
		}
	}()

	srv := &http.Server{
		Addr:    params.listenAddress,
		Handler: &ra,
	}
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for {
			<-ticker.C
			ra.latencyQuery.Tick()
			ra.latencyStats.Tick()
			ra.rateQuery.Tick()
			ra.rateStats.Tick()
			ra.rateTightLoop.Tick()
			ra.statistics.tick1s++
		}
	}()

	go func() {
		for {
			timer100ms := time.NewTimer(100 * time.Millisecond)
			<-timer100ms.C
			ra.statistics.timer100ms++
		}
	}()

	go func() {
		glog.Fatal(srv.ListenAndServe().Error())
	}()
	glog.Infof("Listen on interface %s", srv.Addr)

	for {
		time.Sleep(10 * time.Millisecond)
		ra.statistics.sleep10ms++
	}
}
