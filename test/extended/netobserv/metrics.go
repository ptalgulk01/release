package netobserv

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

// prometheusQueryResult the response of querying prometheus APIs
type prometheusQueryResult struct {
	Data struct {
		Result     []metric `json:"result"`
		ResultType string   `json:"resultType"`
	} `json:"data"`
	Status string `json:"status"`
}

// metric the prometheus metric
type metric struct {
	Metric struct {
		Name          string `json:"__name__"`
		Cluster       string `json:"cluster,omitempty"`
		Container     string `json:"container,omitempty"`
		ContainerName string `json:"containername,omitempty"`
		Endpoint      string `json:"endpoint,omitempty"`
		Instance      string `json:"instance,omitempty"`
		Job           string `json:"job,omitempty"`
		Namespace     string `json:"namespace,omitempty"`
		Path          string `json:"path,omitempty"`
		Pod           string `json:"pod,omitempty"`
		PodName       string `json:"podname,omitempty"`
		Service       string `json:"service,omitempty"`
	} `json:"metric"`
	Value []interface{} `json:"value"`
}

func getMetric(oc *exutil.CLI, query string) ([]metric, error) {
	res, err := queryPrometheus(oc, "/api/v1/query", query, "GET")
	attempts := 10
	for len(res.Data.Result) == 0 && attempts >= 0 {
		if err != nil {
			return []metric{}, err
		}
		time.Sleep(5 * time.Second)
		res, err = queryPrometheus(oc, "/api/v1/query", query, "GET")
		attempts--
	}
	errMsg := fmt.Sprintf("0 results returned for query %s", query)
	o.Expect(len(res.Data.Result)).Should(o.BeNumerically(">=", 1), errMsg)
	return res.Data.Result, nil
}

// queryPrometheus returns the promtheus metrics which match the query string
// path: the api path, for example: /api/v1/query?
// query: the metric or alert you want to search
// action: it can be "GET", "get", "Get", "POST", "post", "Post"
func queryPrometheus(oc *exutil.CLI, path string, query string, action string) (*prometheusQueryResult, error) {
	var bearerToken string
	var err error
	bearerToken = getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

	address := "https://" + getRouteAddress(oc, "openshift-monitoring", "prometheus-k8s")

	h := make(http.Header)
	h.Add("Content-Type", "application/json")
	h.Add("Authorization", "Bearer "+bearerToken)

	params := url.Values{}
	if len(query) > 0 {
		params.Add("query", query)
	}

	var p prometheusQueryResult
	resp, err := doHTTPRequest(h, address, path, params.Encode(), action, false, 5, nil, 200)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(resp, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// return the first metric value
func popMetricValue(metrics []metric) int {
	valInterface := metrics[0].Value[1]
	val, _ := valInterface.(string)
	value, err := strconv.ParseFloat(val, 64)
	o.Expect(err).NotTo(o.HaveOccurred())
	return int(math.Round(value))
}

// verify FLP metrics
func verifyFLPMetrics(oc *exutil.CLI) {
	metrics, err := getMetric(oc, "sum(netobserv_ingest_flows_processed)")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(popMetricValue(metrics)).Should(o.BeNumerically(">", 0))

	metrics, err = getMetric(oc, "sum(netobserv_loki_sent_entries_total)")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(popMetricValue(metrics)).Should(o.BeNumerically(">", 0))
}

func getMetricsScheme(oc *exutil.CLI, servicemonitor string) (string, error) {
	out, err := oc.AsAdmin().Run("get").Args("servicemonitor", servicemonitor, "-n", oc.Namespace(), "-o", "jsonpath='{.spec.endpoints[].scheme}'").Output()

	return out, err
}

func getMetricsServerName(oc *exutil.CLI, servicemonitor string) (string, error) {
	out, err := oc.AsAdmin().Run("get").Args("servicemonitor", servicemonitor, "-n", oc.Namespace(), "-o", "jsonpath='{.spec.endpoints[].tlsConfig.serverName}'").Output()

	return out, err
}
