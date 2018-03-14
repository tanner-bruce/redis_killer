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
	"os"
	"os/signal"
	"sync"

	"github.com/garyburd/redigo/redis"
	"github.com/spf13/cobra"

	log "github.com/sirupsen/logrus"
)

var (
	host        string
	port        int
	jobs        int
	numRequests int
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
	// Handle signal interrupts nicely
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer func() {
		signal.Stop(c)
		cancel()
	}()

	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Connect to redis
	r, err := redis.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		log.WithError(err).Errorln("could not connect to redis")
		os.Exit(1)
	}
	defer r.Close()

	if numRequests < 0 {
		numRequests = 10000000000 // im gonna kill u!!!
	}

	var wg sync.WaitGroup
	for i := 0; i < jobs; i++ {
		go func(id int, c context.Context) {
			wg.Add(1)
			defer wg.Done()

			count := 0
			for ; count < numRequests/jobs; count++ {
				select {
				case <-c.Done():
					return
				default:
				}

				r.Do("SET", string(count*id), count)
			}

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

func init() {
	rootCmd.Flags().StringVarP(&host, "host", "h", "127.0.0.1", "Redis host/ip")
	rootCmd.Flags().IntVarP(&port, "port", "p", 6379, "Redis port")
	rootCmd.Flags().IntVarP(&jobs, "jobs", "j", 4, "Number of parallel jobs to run")
	rootCmd.Flags().IntVarP(&numRequests, "requests", "r", -1, "Number of total requests to make. Set to -1 to run forever")
}
