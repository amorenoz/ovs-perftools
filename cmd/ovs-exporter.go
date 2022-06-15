package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"

	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"

	kexec "k8s.io/utils/exec"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vishvananda/netlink"
	"k8s.io/klog"
)

const (
	MetricOvsNamespace         = "ovs"
	MetricOvsSubsystemVswitchd = "vswitchd"
)

var (
	secs         = flag.Int("period", 30, "Update period in secs")
	period       time.Duration
	appctlPath   string
	ovsOfctlPath string
	ovsVsctlPath string
	kexecIface   kexec.Interface
)

/* Borrowed from ovn-kubernetes/go-controller/pkg/metrics/ovs.go */
var metricOvsDpIfTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "dp_if_total",
	Help:      "Represents the number of ports connected to the datapath."},
	[]string{
		"datapath",
	},
)

var metricOvsDpIf = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "dp_if",
	Help: "A metric with a constant '1' value labeled by " +
		"datapath name, port name, port type and datapath port number."},
	[]string{
		"datapath",
		"port",
		"type",
		"ofPort",
	},
)

var metricOvsDpFlowsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "dp_flows_total",
	Help:      "Represents the number of flows in datapath."},
	[]string{
		"datapath",
	},
)

var metricOvsDpFlowsLookupHit = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "dp_flows_lookup_hit",
	Help: "Represents number of packets matching the existing flows " +
		"while processing incoming packets in the datapath."},
	[]string{
		"datapath",
	},
)

var metricOvsDpFlowsLookupMissed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "dp_flows_lookup_missed",
	Help: "Represents the number of packets not matching any existing " +
		"flow  and require  user space processing."},
	[]string{
		"datapath",
	},
)

var metricOvsDpFlowsLookupLost = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "dp_flows_lookup_lost",
	Help: "number of packets destined for user space process but " +
		"subsequently dropped before  reaching  userspace."},
	[]string{
		"datapath",
	},
)

var metricOvsDpPacketsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "dp_packets_total",
	Help: "Represents the total number of packets datapath processed " +
		"which is the sum of hit and missed."},
	[]string{
		"datapath",
	},
)

var metricOvsdpMasksHit = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "dp_masks_hit",
	Help:      "Represents the total number of masks visited for matching incoming packets.",
},
	[]string{
		"datapath",
	},
)

var metricOvsDpMasksTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "dp_masks_total",
	Help:      "Represents the number of masks in a datapath."},
	[]string{
		"datapath",
	},
)

var metricOvsDpMasksHitRatio = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "dp_masks_hit_ratio",
	Help: "Represents the average number of masks visited per packet " +
		"the  ratio between hit and total number of packets processed by the datapath."},
	[]string{
		"datapath",
	},
)

// ovs bridge statistics & attributes metrics
var metricOvsBridgeTotal = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "bridge_total",
	Help:      "Represents total number of OVS bridges on the system.",
},
)

var metricOvsBridge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "bridge",
	Help: "A metric with a constant '1' value labeled by bridge name " +
		"present on the instance."},
	[]string{
		"bridge",
	},
)

var metricOvsBridgePortsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "bridge_ports_total",
	Help:      "Represents the number of OVS ports on the bridge."},
	[]string{
		"bridge",
	},
)

var metricOvsBridgeFlowsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "bridge_flows_total",
	Help:      "Represents the number of OpenFlow flows on the OVS bridge."},
	[]string{
		"bridge",
	},
)

// ovs memory metrics
var metricOvsHandlersTotal = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "handlers_total",
	Help: "Represents the number of handlers thread. This thread reads upcalls from dpif, " +
		"forwards each upcall's packet and possibly sets up a kernel flow as a cache.",
})

var metricOvsRevalidatorsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: MetricOvsNamespace,
	Subsystem: MetricOvsSubsystemVswitchd,
	Name:      "revalidators_total",
	Help: "Represents the number of revalidators thread. This thread processes datapath flows, " +
		"updates OpenFlow statistics, and updates or removes them if necessary.",
})

//Define a struct for you collector that contains pointers
//to prometheus descriptors for each metric you wish to expose.
//Note you can also include fields of other types if they provide utility
//but we just won't be exposing them as metrics.
type ovsCollector struct {
	ovsMetric *prometheus.Desc
	barMetric *prometheus.Desc
}

//You must create a constructor for you collector that
//initializes every descriptor and returns a pointer to the collector
func newOVSCollector() *ovsCollector {
	return &ovsCollector{
		ovsMetric: prometheus.NewDesc("ovs_metric",
			"Shows whether a ovs has occurred in our cluster",
			nil, nil,
		),
		barMetric: prometheus.NewDesc("bar_metric",
			"Shows whether a bar has occurred in our cluster",
			nil, nil,
		),
	}
}

//Each and every collector must implement the Describe function.
//It essentially writes all descriptors to the prometheus desc channel.
func (collector *ovsCollector) Describe(ch chan<- *prometheus.Desc) {

	//Update this section with the each metric you create for a given collector
	ch <- collector.ovsMetric
	ch <- collector.barMetric
}

//Collect implements required collect function for all promehteus collectors
func (collector *ovsCollector) Collect(ch chan<- prometheus.Metric) {

	//Implement logic here to determine proper metric value to return to prometheus
	//for each descriptor or call other functions that do so.
	var metricValue float64
	if 1 == 1 {
		metricValue += rand.Float64()
	}

	//Write latest value for each metric in the prometheus metric channel.
	//Note that you can pass CounterValue, GaugeValue, or UntypedValue types here.
	m1 := prometheus.MustNewConstMetric(collector.ovsMetric, prometheus.GaugeValue, metricValue)
	m2 := prometheus.MustNewConstMetric(collector.barMetric, prometheus.GaugeValue, metricValue)
	m1 = prometheus.NewMetricWithTimestamp(time.Now().Add(-time.Hour), m1)
	m2 = prometheus.NewMetricWithTimestamp(time.Now(), m2)
	ch <- m1
	ch <- m2
}

func RunCmd(cmdName string, cmdArgs ...string) (string, string, error) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := kexecIface.Command(cmdName, cmdArgs...)
	cmd.SetStdout(stdout)
	cmd.SetStderr(stderr)

	klog.V(2).Infof("appctl %s", strings.Join(cmdArgs, " "))
	err := cmd.Run()
	klog.V(2).Infof("stdout %s", stdout)
	klog.V(2).Infof("stderr %s", stderr)

	return strings.Trim(strings.TrimSpace(stdout.String()), "\""), stderr.String(), err
}
func RunOvsOfctl(args ...string) (string, string, error) {
	return RunCmd(ovsOfctlPath, args...)
}

func RunOvsVsctl(args ...string) (string, string, error) {
	return RunCmd(ovsVsctlPath, args...)
}

// RunOvsAppCtl runs an 'ovs-appctl -t /var/run/openvsiwthc/ovs-vswitchd.pid.ctl command'
func RunOvsAppCtl(args ...string) (string, string, error) {
	//var cmdArgs []string
	//pid, err := afero.ReadFile(AppFs, savedOVSRunDir+"ovs-vswitchd.pid")
	//if err != nil {
	//	return "", "", fmt.Errorf("failed to get ovs-vswitch pid : %v", err)
	//}
	//cmdArgs = []string{
	//	"-t",
	//	savedOVSRunDir + fmt.Sprintf("ovs-vswitchd.%s.ctl", strings.TrimSpace(string(pid))),
	//}
	return RunCmd(appctlPath, args...)
}

func parseMetricToFloat(componentName, metricName, value string) float64 {
	f64Value, err := strconv.ParseFloat(value, 64)
	if err != nil {
		klog.Error("Failed to parse value %s into float for metric %s_%s :(%v)",
			value, componentName, metricName, err)
		return 0
	}
	return f64Value
}

// setOvsMemoryMetrics updates the handlers, revalidators
// count from "ovs-appctl -t ovs-vswitchd memory/show" output.
func setOvsMemoryMetrics() (err error) {
	var stdout, stderr string

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovering from panic while parsing the ovs-appctl "+
				"memory/show output : %v", r)
		}
	}()

	stdout, stderr, err = RunOvsAppCtl("memory/show")
	if err != nil {
		return fmt.Errorf("failed to retrieve memory/show output "+
			"for ovs-vswitchd stderr(%s) :%v", stderr, err)
	}

	for _, kvPair := range strings.Fields(stdout) {
		if strings.HasPrefix(kvPair, "handlers:") {
			value := strings.Split(kvPair, ":")[1]
			count := parseMetricToFloat(MetricOvsSubsystemVswitchd, "handlers_total", value)
			metricOvsHandlersTotal.Set(count)
		} else if strings.HasPrefix(kvPair, "revalidators:") {
			value := strings.Split(kvPair, ":")[1]
			count := parseMetricToFloat(MetricOvsSubsystemVswitchd, "revalidators_total", value)
			metricOvsRevalidatorsTotal.Set(count)
		}
	}
	return nil
}
func ovsMemoryMetricsUpdate() {
	for {
		err := setOvsMemoryMetrics()
		if err != nil {
			klog.Error("%s", err.Error())
		}
		time.Sleep(period)
	}
}

// getOvsDatapaths gives list of datapaths
// and updates the corresponding datapath metrics
func getOvsDatapaths() (datapathsList []string, err error) {
	var stdout, stderr string

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovering from a panic while parsing the "+
				"ovs-appctl dpctl/dump-dps output : %v", r)
		}
	}()

	stdout, stderr, err = RunOvsAppCtl("dpctl/dump-dps")
	if err != nil {
		return nil, fmt.Errorf("failed to get output of ovs-dpctl dump-dps "+
			"stderr(%s) :(%v)", stderr, err)
	}
	for _, kvPair := range strings.Split(stdout, "\n") {
		var datapathName string
		output := strings.TrimSpace(kvPair)
		if strings.Contains(output, "@") {
			datapath := strings.Split(output, "@")
			_, datapathName = datapath[0], datapath[1]
		} else {
			return nil, fmt.Errorf("datapath %s is not of format Type@Name", output)
		}
		datapathsList = append(datapathsList, datapathName)
	}
	return datapathsList, nil
}

// ovsDatapathLookupsMetrics obtains the ovs datapath
// (lookups: hit, missed, lost) metrics and updates them.
func ovsDatapathLookupsMetrics(output, datapath string) {
	var datapathPacketsTotal float64
	for _, field := range strings.Fields(output) {
		elem := strings.Split(field, ":")
		if len(elem) != 2 {
			continue
		}
		switch elem[0] {
		case "hit":
			value := parseMetricToFloat(MetricOvsSubsystemVswitchd, "dp_flows_lookup_hit", elem[1])
			datapathPacketsTotal += value
			metricOvsDpFlowsLookupHit.WithLabelValues(datapath).Set(value)
		case "missed":
			value := parseMetricToFloat(MetricOvsSubsystemVswitchd, "dp_flows_lookup_missed", elem[1])
			datapathPacketsTotal += value
			metricOvsDpFlowsLookupMissed.WithLabelValues(datapath).Set(value)
		case "lost":
			value := parseMetricToFloat(MetricOvsSubsystemVswitchd, "dp_flows_lookup_lost", elem[1])
			metricOvsDpFlowsLookupLost.WithLabelValues(datapath).Set(value)
		}
	}
	metricOvsDpPacketsTotal.WithLabelValues(datapath).Set(datapathPacketsTotal)
}

// ovsDatapathMasksMetrics obatins ovs datapath masks metrics
// (masks :hit, total, hit/pkt) and updates them.
func ovsDatapathMasksMetrics(output, datapath string) {
	for _, field := range strings.Fields(output) {
		elem := strings.Split(field, ":")
		if len(elem) != 2 {
			continue
		}
		switch elem[0] {
		case "hit":
			value := parseMetricToFloat(MetricOvsSubsystemVswitchd, "dp_masks_hit", elem[1])
			metricOvsdpMasksHit.WithLabelValues(datapath).Set(value)
		case "total":
			value := parseMetricToFloat(MetricOvsSubsystemVswitchd, "dp_masks_total", elem[1])
			metricOvsDpMasksTotal.WithLabelValues(datapath).Set(value)
		case "hit/pkt":
			value := parseMetricToFloat(MetricOvsSubsystemVswitchd, "dp_masks_hit_ratio", elem[1])
			metricOvsDpMasksHitRatio.WithLabelValues(datapath).Set(value)
		}
	}
}

// ovsDatapathPortMetrics obtains the ovs datapath port metrics
// from ovs-appctl dpctl/show(portname, porttype, portnumber) and updates them.
func ovsDatapathPortMetrics(output, datapath string) {
	portFields := strings.Fields(output)
	portType := "system"
	if len(portFields) > 3 {
		portType = strings.Trim(portFields[3], "():")
	}

	portName := strings.TrimSpace(portFields[2])
	portNumber := strings.Trim(portFields[1], ":")
	metricOvsDpIf.WithLabelValues(datapath, portName, portType, portNumber).Set(1)
}

func setOvsDatapathMetrics(datapaths []string) (err error) {
	var stdout, stderr, datapathName string

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovering from a panic while parsing the ovs-dpctl "+
				"show %s output : %v", datapathName, r)
		}
	}()

	for _, datapathName = range datapaths {
		stdout, stderr, err = RunOvsAppCtl("dpctl/show", datapathName)
		if err != nil {
			return fmt.Errorf("failed to get datapath stats for %s "+
				"stderr(%s) :(%v)", datapathName, stderr, err)
		}
		var datapathPortCount float64
		for i, kvPair := range strings.Split(stdout, "\n") {
			if i <= 0 {
				// skip the first line which is datapath name
				continue
			}
			output := strings.TrimSpace(kvPair)
			if strings.HasPrefix(output, "lookups:") {
				ovsDatapathLookupsMetrics(output, datapathName)
			} else if strings.HasPrefix(output, "masks:") {
				ovsDatapathMasksMetrics(output, datapathName)
			} else if strings.HasPrefix(output, "port ") {
				ovsDatapathPortMetrics(output, datapathName)
				datapathPortCount++
			} else if strings.HasPrefix(output, "flows:") {
				flowFields := strings.Fields(output)
				value := parseMetricToFloat(MetricOvsSubsystemVswitchd, "dp_flows_total", flowFields[1])
				metricOvsDpFlowsTotal.WithLabelValues(datapathName).Set(value)
			}
		}
		metricOvsDpIfTotal.WithLabelValues(datapathName).Set(datapathPortCount)
	}
	return nil
}

// ovsDatapathMetricsUpdate updates the ovs datapath metrics for every 30 sec
func ovsDatapathMetricsUpdate() {
	for {
		time.Sleep(period)
		datapaths, err := getOvsDatapaths()
		if err != nil {
			klog.Errorf("%s", err.Error())
			continue
		}

		err = setOvsDatapathMetrics(datapaths)
		if err != nil {
			klog.Errorf("%s", err.Error())
		}
	}
}

type ovsInterfaceMetricsDetails struct {
	help   string
	metric *prometheus.GaugeVec
}

var ovsInterfaceMetricsDataMap = map[string]*ovsInterfaceMetricsDetails{
	"interface_rx_packets": {
		help: "Represents the number of received packets " +
			"by OVS interface.",
	},
	"interface_rx_bytes": {
		help: "Represents the number of received bytes by " +
			"OVS interface.",
	},
	"interface_rx_dropped": {
		help: "Represents the number of input packets dropped " +
			"by OVS interface.",
	},
	"interface_rx_frame_err": {
		help: "Represents the number of frame alignment errors " +
			"on the packets received by OVS interface.",
	},
	"interface_rx_over_err": {
		help: "Represents the number of packets with RX overrun " +
			"received by OVS interface.",
	},
	"interface_rx_crc_err": {
		help: "Represents the number of CRC errors for the packets " +
			"received by OVS interface.",
	},
	"interface_rx_errors": {
		help: "Represents the total number of packets with errors " +
			"received by OVS interface.",
	},
	"interface_tx_packets": {
		help: "Represents the number of transmitted packets by " +
			"OVS interface.",
	},
	"interface_tx_bytes": {
		help: "Represents the number of transmitted bytes " +
			"by OVS interface.",
	},
	"interface_tx_dropped": {
		help: "Represents the number of output packets dropped " +
			"by OVS interface.",
	},
	"interface_collisions": {
		help: "Represents the number of collisions " +
			"on the packets transmitted by OVS interface.",
	},
	"interface_tx_errors": {
		help: "Represents the total number of packets with errors " +
			"transmitted by OVS interface.",
	},
	"interface_ingress_policing_rate": {
		help: "Maximum rate for data received on OVS interface, " +
			"in kbps. If the value is 0, then policing is disabled.",
	},
	"interface_ingress_policing_burst": {
		help: "Maximum burst size for data received on OVS interface, " +
			"in kb. The default burst size if set to 0 is 8000 kbit.",
	},
	"interface_admin_state": {
		help: "The administrative state of the OVS interface. " +
			"The values are: other(0), down(1) or up(2).",
	},
	"interface_link_state": {
		help: "The link state of the OVS interface. " +
			"The values are: down(1) or up(2) or other(0).",
	},
	"interface_type": {
		help: "Represents the interface type other(0), system(1), internal(2), " +
			"tap(3), geneve(4), gre(5), vxlan(6), lisp(7), stt(8), patch(9).",
	},
	"interface_mtu": {
		help: "The currently configured MTU for OVS interface.",
	},
	"interface_of_port": {
		help: "Represents the OpenFlow port ID associated with OVS interface.",
	},
	"interface_duplex": {
		help: "The duplex mode of the OVS interface. The values are half(0) " +
			"or full(1) or other(2)",
	},
	"interface_ifindex": {
		help: "Represents the interface index associated with OVS interface.",
	},
	"interface_link_speed": {
		help: "The negotiated speed of the OVS interface.",
	},
	"interface_link_resets": {
		help: "The number of times Open vSwitch has observed the " +
			"link_state of OVS interface change.",
	},
}

func getOvsInterfaceState(state string) float64 {
	var stateValue float64
	if state == "" {
		return 0
	}
	stateMap := map[string]float64{
		"down": 1,
		"up":   2,
	}
	if value, ok := stateMap[state]; ok {
		stateValue = value
	} else {
		stateValue = 0
	}
	return stateValue
}

func getGeneveInterfaceStatsFieldValue(stats *netlink.LinkStatistics, field string) float64 {
	r := reflect.ValueOf(stats)
	fieldValue := reflect.Indirect(r).FieldByName(field)
	return float64(fieldValue.Uint())
}

func setGeneveInterfaceStatistics(geneveInterfaceName string, link netlink.Link) {
	var geneveInterfaceStatsMap = map[string]string{
		"rx_packets":   "RxPackets",
		"rx_bytes":     "RxBytes",
		"rx_dropped":   "RxDropped",
		"rx_frame_err": "RxFrameErrors",
		"rx_over_err":  "RxOverErrors",
		"rx_crc_err":   "RxCrcErrors",
		"rx_errors":    "RxErrors",
		"tx_packets":   "TxPackets",
		"tx_bytes":     "TxBytes",
		"tx_dropped":   "TxDropped",
		"collisions":   "Collisions",
		"tx_errors":    "TxErrors",
	}

	for statsName, geneveStatsName := range geneveInterfaceStatsMap {
		metricName := "interface_" + statsName
		metricValue := getGeneveInterfaceStatsFieldValue(link.Attrs().Statistics, geneveStatsName)
		ovsInterfaceMetricsDataMap[metricName].metric.WithLabelValues(
			"none", "none", geneveInterfaceName).Set(metricValue)
	}
}

// geneveInterfaceMetricsUpdate updates the geneve interface
// metrics obtained through netlink library equivalent to
// (ip -s li show genev_sys_6081)
func geneveInterfaceMetricsUpdate() error {
	geneveInterfaceName := "genev_sys_6081"
	link, err := netlink.LinkByName(geneveInterfaceName)
	if err != nil {
		return fmt.Errorf("failed to lookup link %s: (%v)", geneveInterfaceName, err)
	}
	ovsInterfaceMetricsDataMap["interface_mtu"].metric.WithLabelValues(
		"none", "none", geneveInterfaceName).Set(float64(link.Attrs().MTU))
	geneveInterfaceLinkStateValue := getOvsInterfaceState(link.Attrs().OperState.String())
	ovsInterfaceMetricsDataMap["interface_link_state"].metric.WithLabelValues(
		"none", "none", geneveInterfaceName).Set(geneveInterfaceLinkStateValue)
	ovsInterfaceMetricsDataMap["interface_ifindex"].metric.WithLabelValues(
		"none", "none", geneveInterfaceName).Set(float64(link.Attrs().Index))
	setGeneveInterfaceStatistics(geneveInterfaceName, link)
	return nil
}

// getOvsBridgeOpenFlowsCount returns the number of openflow flows
// in an ovs-bridge
func getOvsBridgeOpenFlowsCount(bridgeName string) float64 {
	stdout, stderr, err := RunOvsOfctl("-t", "5", "dump-aggregate", bridgeName)
	if err != nil {
		klog.Errorf("Failed to get flow count for %s, stderr(%s): (%v)",
			bridgeName, stderr, err)
		return 0
	}
	for _, kvPair := range strings.Fields(stdout) {
		if strings.HasPrefix(kvPair, "flow_count=") {
			value := strings.Split(kvPair, "=")[1]
			metricName := bridgeName + "flows_total"
			return parseMetricToFloat(MetricOvsSubsystemVswitchd, metricName, value)
		}
	}
	klog.Errorf("ovs-ofctl dump-aggregate %s output didn't contain "+
		"flow_count field", bridgeName)
	return 0
}

// getOvsBridgeInfo obtains the (per Brdige port count) &
// port to bridge mapping for each port
func getOvsBridgeInfo() (bridgePortCount map[string]float64, portToBridgeMap map[string]string,
	err error) {
	var stdout, stderr string

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovering from a panic while parsing the "+
				"ovs-vsctl list Bridge output : %v", r)
		}
	}()

	stdout, stderr, err = RunOvsVsctl("--no-headings", "--data=bare",
		"--format=csv", "--columns=name,port", "list", "Bridge")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get output for ovs-vsctl list Bridge "+
			"stderr(%s) :(%v)", stderr, err)
	}

	bridgePortCount = make(map[string]float64)
	portToBridgeMap = make(map[string]string)
	//output will be of format :(br-local,12bc8575-8e1f-4583-b693-ea3b5bf09974
	// 5dc87c46-4d94-4469-9f7a-67ee1c8beb03 620cafe4-bfe5-4a23-8165-4ffc61e7de42)
	for _, kvPair := range strings.Split(stdout, "\n") {
		if kvPair == "" {
			continue
		}
		fields := strings.Split(kvPair, ",")
		bridgeName := fields[0]
		ports := strings.Fields(fields[1])
		if bridgeName != "" {
			bridgePortCount[bridgeName] = float64(len(ports))
		}
		for _, portId := range ports {
			portToBridgeMap[portId] = bridgeName
		}
	}
	return bridgePortCount, portToBridgeMap, nil
}

// ovsBridgeMetricsUpdate updates bridgeMetrics &
// ovsInterface metrics & geneveInterface metrics for every 30sec
func ovsBridgeMetricsUpdate() {
	for {
		time.Sleep(period)
		// set geneve interface metrics
		err := geneveInterfaceMetricsUpdate()
		if err != nil {
			klog.Errorf("%s", err.Error())
		}
		// update ovs bridge metrics
		bridgePortCountMapping, _, err := getOvsBridgeInfo()
		if err != nil {
			klog.Errorf("%s", err.Error())
			continue
		}
		for brName, nPorts := range bridgePortCountMapping {
			metricOvsBridge.WithLabelValues(brName).Set(1)
			metricOvsBridgePortsTotal.WithLabelValues(brName).Set(nPorts)
			flowsCount := getOvsBridgeOpenFlowsCount(brName)
			metricOvsBridgeFlowsTotal.WithLabelValues(brName).Set(flowsCount)
		}
		metricOvsBridgeTotal.Set(float64(len(bridgePortCountMapping)))

		//interfaceToPortToBridgeMap, err := getInterfaceToPortToBridgeMapping(portBridgeMapping)
		//if err != nil {
		//	klog.Errorf("%s", err.Error())
		//	continue
		//}
		// set ovs interface metrics.
		//err = ovsInterfaceMetricsUpdate(interfaceToPortToBridgeMap)
		//if err != nil {
		//	klog.Errorf("%s", err.Error())
		//}
	}
}

func main() {
	flag.Parse()
	period = time.Duration(*secs) * time.Second
	var err error
	appctlPath, err = exec.LookPath("ovs-appctl")
	if err != nil {
		log.Fatal("no ovs-appctl found in path")
	}
	ovsOfctlPath, err = exec.LookPath("ovs-ofctl")
	if err != nil {
		log.Fatal("no ovs-ofctl found in path")
	}
	ovsVsctlPath, err = exec.LookPath("ovs-vsctl")
	if err != nil {
		log.Fatal("no ovs-ofctl found in path")
	}
	kexecIface = kexec.New()
	prometheus.MustRegister(metricOvsDpIfTotal)
	prometheus.MustRegister(metricOvsDpIf)
	prometheus.MustRegister(metricOvsDpFlowsTotal)
	prometheus.MustRegister(metricOvsDpFlowsLookupHit)
	prometheus.MustRegister(metricOvsDpFlowsLookupMissed)
	prometheus.MustRegister(metricOvsDpFlowsLookupLost)
	prometheus.MustRegister(metricOvsDpPacketsTotal)
	prometheus.MustRegister(metricOvsdpMasksHit)
	prometheus.MustRegister(metricOvsDpMasksTotal)
	prometheus.MustRegister(metricOvsDpMasksHitRatio)
	// Register OVS bridge statistics & attributes metrics
	prometheus.MustRegister(metricOvsBridgeTotal)
	prometheus.MustRegister(metricOvsBridge)
	prometheus.MustRegister(metricOvsBridgePortsTotal)
	prometheus.MustRegister(metricOvsBridgeFlowsTotal)
	// Register ovs Memory metrics
	prometheus.MustRegister(metricOvsHandlersTotal)
	prometheus.MustRegister(metricOvsRevalidatorsTotal)
	// Register OVS HW offload metrics

	// OVS datapath metrics updater
	go ovsDatapathMetricsUpdate()
	// OVS memory metrics updater
	go ovsMemoryMetricsUpdate()
	// OVS bridge metrics updater
	go ovsBridgeMetricsUpdate()

	http.Handle("/console/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":9101", nil))

}
