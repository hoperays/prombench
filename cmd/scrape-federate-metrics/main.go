package main

import (
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/prometheus/promql/parser"
)

const (
	federatePath       = "/federate"
	MatcherParam       = "match[]"
	DefaultInterval    = time.Duration(20) * time.Second
	DefaultTimeout     = time.Duration(19) * time.Second
	scrapeAcceptHeader = `application/openmetrics-text;version=1.0.0,application/openmetrics-text;version=0.0.1;q=0.75,text/plain;version=0.0.4;q=0.5,*/*;q=0.1`
)

// prometheus metrics
var (
	scrapeUpstream = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "federate_scrape_upstream",
			Help: "Scrape upstream",
		},
		[]string{"path"},
	)
	scrapeIntervalConf = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "federate_scrape_interval_seconds",
			Help: "Scrape interval",
		},
	)
	scrapeTimeoutConf = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "federate_scrape_timeout_seconds",
			Help: "Scrape timeout",
		},
	)
	scrapeParallelNum = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "federate_scrape_paralles",
			Help: "Number of parallel scrape",
		},
	)
	scrapeDurations = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "federate_scrape_duration",
			Help: "Duration (seconds) of scrape requests",
		},
		[]string{"query", "status_code"},
	)
	// scrapeDurationsBuckets = prometheus.NewHistogramVec(
	// 	prometheus.HistogramOpts{
	// 		Name:    "federate_scrape_duration_bucket",
	// 		Help:    "Duration of scrape requests with response code",
	// 		Buckets: []float64{0.01, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 20, 30, 50},
	// 	},
	// 	[]string{"status_code", "query"},
	// )
	scrapeBodySize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "federate_scrape_body_size_bytes",
			Help: "Body size of scrape responses",
		},
		[]string{"query"},
	)
	scrapeLines = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "federate_scrape_lines",
			Help: "Count of scrape lines",
		},
		[]string{"query"},
	)
)

type arrayFlags []string

func (i *arrayFlags) String() string {
	return "Nothing"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var (
	listenAddress  string
	upstreamPath   string
	matcherParams  arrayFlags
	userName       string
	userPassword   string
	scrapeInterval time.Duration
	scrapeTimeout  time.Duration
	flagset        = flag.CommandLine
)

type ScrapeClient struct {
	Client         *http.Client
	Req            *http.Request
	MatcherEncoded []string
}

func NewScrapeClient() (ScrapeClient, error) {
	scrapeUpstream.With(prometheus.Labels{"path": upstreamPath}).Set(1)
	scrapeIntervalConf.Set(scrapeInterval.Seconds())
	scrapeTimeoutConf.Set(scrapeTimeout.Seconds())

	if upstreamPath == "" {
		return ScrapeClient{}, fmt.Errorf("upstream cannot be empty")
	}

	target, err := url.Parse(upstreamPath)
	if err != nil {
		return ScrapeClient{}, fmt.Errorf("upstream path %s parse error", upstreamPath)
	}

	if len(matcherParams) < 1 {
		return ScrapeClient{}, fmt.Errorf("at least one matcher needs to be specified")
	}

	matcherList, err := parseMatchers()
	if err != nil {
		return ScrapeClient{}, err
	}

	scrapeParallelNum.Set(float64(len(matcherList)))
	log.Printf("Total %d matchers", len(matcherList))

	if target.Path == "" {
		target.Path = federatePath
	}

	log.Printf("Scrape upstream: %s", target.String())
	if userName != "" && userPassword != "" {
		target.User = url.UserPassword(userName, userPassword)
	}

	req, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		return ScrapeClient{}, fmt.Errorf("http new request error: %s", err)
	}

	req.Header.Add("Accept", scrapeAcceptHeader)
	req.Header.Add("Accept-Encoding", "gzip")

	sClient := ScrapeClient{
		Client:         &http.Client{},
		Req:            req,
		MatcherEncoded: matcherList,
	}

	return sClient, nil
}

func (sc ScrapeClient) ScrapeFederate(index int, matcher string) {
	// Set timeout.
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), scrapeTimeout)
	defer cancel()

	req := sc.Req.Clone(ctx)
	req.URL.RawQuery = sc.MatcherEncoded[index]
	resp, err := sc.Client.Do(req)

	responseTime := time.Since(start).Seconds()
	if err != nil {
		reportMetrics(matcher, 500, 0, 0, responseTime)
		log.Printf("Scrape %s federate api error: %s", matcher, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		reportMetrics(matcher, resp.StatusCode, 0, 0, responseTime)
		log.Printf("Scrape %s returned http status %s", matcher, resp.Status)
		return
	}

	var reader io.ReadCloser
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			log.Printf("Scrape %s gzip response body read error: %s", matcher, err)
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("Scrape %s response body parse error: %s", matcher, err)
	}

	lines := len(strings.Split(strings.TrimSpace(string(body)), "\n"))
	reportMetrics(matcher, resp.StatusCode, len(body), lines, responseTime)
}

func parseMatchers() ([]string, error) {
	matcherList := []string{}
	for _, m := range matcherParams {
		querys, err := url.ParseQuery(m)
		if err != nil {
			return matcherList, fmt.Errorf("matcher %s parse error: %s", m, err)
		}

		if len(querys.Get(MatcherParam)) == 0 {
			return matcherList, fmt.Errorf("matcher %s needs to start with match[]=", m)
		}

		for _, v := range querys[MatcherParam] {
			_, err = parser.ParseMetricSelector(v)
			if err != nil {
				return matcherList, fmt.Errorf("promql expr of matcher %s parse error: %s", m, err)
			}
		}

		// Encode
		matcherList = append(matcherList, querys.Encode())
	}

	return matcherList, nil
}

func reportMetrics(matcher string, code, size, lines int, responseTime float64) {
	log.Printf("Scrape %s end: %d code, %fs, %d bytes, %d lines", matcher, code, responseTime, size, lines)

	scrapeDurations.DeletePartialMatch(prometheus.Labels{"query": matcher})
	scrapeBodySize.DeletePartialMatch(prometheus.Labels{"query": matcher})
	scrapeLines.DeletePartialMatch(prometheus.Labels{"query": matcher})

	// scrapeDurationsBuckets.With(prometheus.Labels{
	// 	"query":       matcher,
	// 	"status_code": fmt.Sprint(code),
	// }).Observe(responseTime)

	scrapeDurations.With(prometheus.Labels{
		"query":       matcher,
		"status_code": fmt.Sprint(code),
	}).Set(responseTime)

	if code != http.StatusOK {
		return
	}

	scrapeBodySize.With(prometheus.Labels{
		"query": matcher,
	}).Set(float64(size))

	scrapeLines.With(prometheus.Labels{
		"query": matcher,
	}).Set(float64(lines))
}

func init() {
	flag.StringVar(&listenAddress, "listen-address", ":9089", "Address on which to expose metrics interface.")
	flag.StringVar(&upstreamPath, "upstream-path", "http://127.0.0.1:9090/federate", "The upstream thanos federate URL")
	flag.Var(&matcherParams, "matcher", "The matcher of thanos federate api, such as match[]=up, can be added repeatedly. Concurrent requests will be made according to the number of mathers.")
	flag.StringVar(&userName, "username", "", "BasicAuth user name")
	flag.StringVar(&userPassword, "password", "", "BasicAuth user password")
	flag.DurationVar(&scrapeInterval, "scrape-interval", DefaultInterval, "Scrape interval.")
	flag.DurationVar(&scrapeTimeout, "scrape-timeout", DefaultTimeout, "Scrape timeout.")

	prometheus.MustRegister(
		scrapeUpstream,
		scrapeIntervalConf,
		scrapeTimeoutConf,
		scrapeParallelNum,
		scrapeDurations,
		// scrapeDurationsBuckets,
		scrapeBodySize,
		scrapeLines,
	)
}

func main() {
	_ = flagset.Parse(os.Args[1:])
	sClient, err := NewScrapeClient()
	if err != nil {
		log.Println(err)
		return
	}

	var wg sync.WaitGroup
	ticker := time.NewTicker(scrapeInterval)
	defer ticker.Stop()

	go func() {
		for {
			<-ticker.C

			for i, m := range matcherParams {
				wg.Add(1)

				go func(i int, m string) {
					sClient.ScrapeFederate(i, m)
					wg.Done()
				}(i, m)
			}

			wg.Wait()
		}
	}()

	log.Printf("Listening /metrics on %s", listenAddress)
	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(listenAddress, nil)
}
