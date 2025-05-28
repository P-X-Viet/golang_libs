package main

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
	"github.com/spf13/viper"
)

// ---------------- CONFIG STRUCT ----------------

type Rule struct {
	And       []Rule `mapstructure:"and"`
	Or        []Rule `mapstructure:"or"`
	Condition string `mapstructure:"condition"`
}

type Alert struct {
	Name string `mapstructure:"name"`
	Rule Rule   `mapstructure:"rule"`
}

type Config struct {
	Alerts []Alert `mapstructure:"alerts"`
}

// ---------------- UNIT PARSER ----------------

func parseWithUnits(val string) (float64, error) {
	unitMap := map[string]float64{
		"b":   1,
		"kib": 1024,
		"mib": 1024 * 1024,
		"gib": 1024 * 1024 * 1024,
	}

	re := regexp.MustCompile(`(?i)([0-9.]+)\s*([a-zA-Z]*)`)
	matches := re.FindStringSubmatch(val)
	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid value format: %s", val)
	}

	num, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, err
	}

	unit := strings.ToLower(matches[2])
	if unit == "" {
		return num, nil
	}

	factor, ok := unitMap[unit]
	if !ok {
		return 0, fmt.Errorf("unknown unit: %s", unit)
	}

	return num * factor, nil
}

// ---------------- RULE EVALUATOR ----------------

func evalRule(rule Rule, metrics map[string]float64, ctx map[string]float64) bool {
	if rule.Condition != "" {
		parts := strings.Fields(rule.Condition)
		if len(parts) != 3 {
			log.Printf("invalid condition: %s", rule.Condition)
			return false
		}

		key := parts[0]
		op := parts[1]
		thresholdStr := parts[2]

		threshold, err := parseWithUnits(thresholdStr)
		if err != nil {
			log.Printf("error parsing value %s: %v", thresholdStr, err)
			return false
		}

		actual, ok := metrics[key]
		if !ok {
			log.Printf("missing metric: %s", key)
			return false
		}

		switch op {
		case ">":
			return actual > threshold
		case "<":
			return actual < threshold
		case ">=":
			return actual >= threshold
		case "<=":
			return actual <= threshold
		case "==":
			return actual == threshold
		case "!=":
			return actual != threshold
		default:
			log.Printf("unknown operator: %s", op)
			return false
		}
	}

	if len(rule.And) > 0 {
		for _, sub := range rule.And {
			if !evalRule(sub, metrics, ctx) {
				return false
			}
		}
		return true
	}

	if len(rule.Or) > 0 {
		for _, sub := range rule.Or {
			if evalRule(sub, metrics, ctx) {
				return true
			}
		}
		return false
	}

	return false
}

// ---------------- SYSTEM METRICS ----------------

func getSystemMetrics() (map[string]float64, error) {
	metrics := make(map[string]float64)

	cpuPercents, err := cpu.Percent(0, false)
	if err != nil || len(cpuPercents) == 0 {
		return nil, err
	}
	cores, err := cpu.Counts(false)
	if err != nil {
		return nil, err
	}
	metrics["cpu"] = (cpuPercents[0] / 100.0) * float64(cores)

	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}
	metrics["memory"] = float64(vm.Used)

	return metrics, nil
}

// ---------------- WEBSOCKET ----------------

type Client struct {
	conn *websocket.Conn
	send chan string
}

var clients = make(map[*Client]bool)
var broadcast = make(chan string)

func handleConnections(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	conn, _ := upgrader.Upgrade(w, r, nil)

	client := &Client{conn: conn, send: make(chan string, 256)}
	clients[client] = true

	go func() {
		defer conn.Close()
		for msg := range client.send {
			conn.WriteMessage(websocket.TextMessage, []byte(msg))
		}
	}()
}

// ---------------- LOOP ----------------

func startEvaluationLoop(cfg Config) {
	for {
		metrics, err := getSystemMetrics()
		if err != nil {
			log.Println("Error collecting metrics:", err)
			continue
		}

		for _, alert := range cfg.Alerts {
			if evalRule(alert.Rule, metrics, nil) {
				broadcast <- alert.Name
			}
		}

		time.Sleep(5 * time.Second)
	}
}

func startBroadcaster() {
	for {
		msg := <-broadcast
		for client := range clients {
			select {
			case client.send <- msg:
			default:
				delete(clients, client)
			}
		}
	}
}

// ---------------- MAIN ----------------

func main() {
	var cfg Config

	v := viper.New()
	v.SetConfigFile("rules.yaml")
	if err := v.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}
	if err := v.Unmarshal(&cfg); err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	go startBroadcaster()
	go startEvaluationLoop(cfg)

	http.HandleFunc("/ws", handleConnections)
	log.Println("WebSocket server on :8080/ws")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
