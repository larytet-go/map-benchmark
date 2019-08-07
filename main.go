package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/larytet-go/accumulator"
	"golang.org/x/sync/syncmap"
)

type restAPI struct {
	params  systemParams
	bigMap  *syncmap.Map
	rate    *accumulator.Accumulator
	latency *accumulator.Accumulator
}

func (ra *restAPI) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	timestamp := time.Now()
	ra.rate.Add(1)
	urlPath := strings.ToLower(request.URL.Path[1:])
	switch urlPath {
	case "query":
		key := request.URL.Query().Get("key")
		if key != "" {
			if valueIfc, ok := ra.bigMap.Load(key); ok {
				fmt.Fprintf(response, "%v\n", valueIfc)
			} else {
				fmt.Fprintf(response, "%v is not found\n", key)
			}
		}
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
		fmt.Fprintf(response, "\n")
		fmt.Fprintf(response, ra.rate.Sprintf("%-28s (requests/s):\n%v\n", "%-28sNo requests in the last %d seconds\n", "%8d ", 16, 1, false))
		fmt.Fprintf(response, "\n")
		fmt.Fprintf(response, ra.latency.Sprintf("%-28s (microseconds):\n%v\n", "%-28sNo requests in the last %d seconds\n", "%8d ", 16, uint64(time.Microsecond), true))
	}
	latency := time.Since(timestamp)
	ra.latency.Add(uint64(latency))
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
	for i := 0; i < count; i++ {
		bigMap.Store(fmt.Sprintf("%d", rand.Uint64()), fmt.Sprintf("%d", rand.Uint64()))
	}
}

func main() {

	params, err := getParams()
	if err != nil {
		glog.Errorf("Failed to parse command line arguments %v", err)
		return
	}
	ra := restAPI{
		params:  params,
		bigMap:  &syncmap.Map{},
		rate:    accumulator.New("rate", 60),
		latency: accumulator.New("latency", 60),
	}
	go func() {
		glog.Infof("Populating map %d entries", params.bigMapSize)
		populateMap(ra.bigMap, params.bigMapSize)
		glog.Infof("Map populated")
	}()

	srv := &http.Server{
		Addr:    params.listenAddress,
		Handler: &ra,
	}
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for {
			<-ticker.C
			ra.latency.Tick()
			ra.rate.Tick()
		}
	}()
	go func() {
		glog.Fatal(srv.ListenAndServe().Error())
	}()
	glog.Infof("Listen on interface %s", srv.Addr)
	readCh := make(chan bool)
	<-readCh
}
