// Copyright © 2018 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/garyburd/redigo/redis"
	"github.com/spf13/cobra"

	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	host        string
	addr        string
	port        int
	jobs        int
	numRequests int
	valueSize   int
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "redis-killer",
	Short: "A brief description of your application",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		run()
	},
}

func run() {
	srv := http.Server{Addr: addr}
	http.Handle("/metrics", promhttp.Handler())

	go func() {
		log.Infoln("starting metrics server")
		if err := srv.ListenAndServe(); err != nil {
			log.WithError(err).Infoln("http metrics server shutting down")
		}
	}()

	log.Infoln("Starting redis killer...")
	// Handle signal interrupts nicely
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		srv.Shutdown(ctx)
		signal.Stop(c)
		cancel()
	}()

	go func() {
		select {
		case <-c:
			srv.Shutdown(ctx)
			cancel()
		case <-ctx.Done():
		}
	}()

	if numRequests < 0 {
		numRequests = 10000000000 // im gonna kill u!!!
	}

	log.Infoln("Starting workers...")
	var wg sync.WaitGroup

	go func(c context.Context) {
		wg.Add(1)
		defer wg.Done()
		var timer *prometheus.Timer
		down := false
		for {
			select {
			case <-c.Done():
				return
			default:
			}

			r, err := redis.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
			if err != nil {
				errCount.WithLabelValues("tester").Inc()
				if !down {
					down = true
					timer = prometheus.NewTimer(downTime.WithLabelValues("tester"))
				}
			} else {
				if down {
					timer.ObserveDuration()
					log.Infoln("tester observed downtime")
					down = false
				}
				r.Close()
			}
		}
	}(ctx)

	for i := 0; i < jobs; i++ {
		wg.Add(1)
		go func(id int, c context.Context) {
			log.Infoln("Connecting to redis...")
			// Connect to redis
			r, err := redis.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
			if err != nil {
				log.WithError(err).Errorln("could not connect to redis")
				os.Exit(1)
			}
			defer r.Close()

			log.Infoln("starting worker... id: ", id)
			defer wg.Done()

			count := 0
			bs := make([]byte, valueSize)
			for ; count < numRequests/jobs; count++ {
				t := prometheus.NewTimer(respTime.WithLabelValues(string(id)))
				select {
				case <-c.Done():
					goto DONE
				default:
				}
				rand.Read(bs)
				_, err := r.Do("SET", string(count*id), bs)
				setCount.WithLabelValues(string(id)).Inc()
				t.ObserveDuration()
				if err != nil {
					errCount.WithLabelValues(string(id)).Inc()
				}
			}
		DONE:
			log.Infof("worker (%d) finished. total requests: %d", id, count)
		}(i, ctx)
	}

	wg.Wait()
	log.Infoln("finished processing")
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var downTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:      "redis_down_time",
	Namespace: "redis_killer",
	Help:      "how long was redis down for",
	Subsystem: "redis",
	Buckets:   []float64{},
},
	[]string{"worker_id"})

var respTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:      "redis_response_time",
	Namespace: "redis_killer",
	Help:      "redis response time",
	Subsystem: "redis",
	Buckets:   []float64{},
},
	[]string{"worker_id"})

var setCount = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name:      "redis_set_total",
	Namespace: "redis_killer",
	Help:      "number of SETs made to redis",
	Subsystem: "redis",
},
	[]string{"worker_id"})

var errCount = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name:      "redis_error_response_total",
	Namespace: "redis_killer",
	Help:      "number of errors counted during SETs made to redis",
	Subsystem: "redis",
},
	[]string{"worker_id"})

func init() {
	prometheus.MustRegister(downTime, respTime, setCount, errCount)

	rootCmd.Flags().StringVar(&host, "host", "127.0.0.1", "Redis host/ip")
	rootCmd.Flags().StringVarP(&addr, "metrics-port", "m", ":9876", "Port to serve metrics on")
	rootCmd.Flags().IntVarP(&port, "port", "p", 6379, "Redis port")
	rootCmd.Flags().IntVarP(&jobs, "jobs", "j", 4, "Number of parallel jobs to run")
	rootCmd.Flags().IntVarP(&numRequests, "requests", "r", -1, "Number of total requests to make. Set to -1 to run forever")
	rootCmd.Flags().IntVarP(&valueSize, "size", "s", -1, "Number of bytes to use for the values inserted. Set to -1 for random")
}
