package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/citadel/citadel"
	"github.com/gorilla/mux"
	"github.com/rcrowley/go-metrics"
)

var (
	configPath     string
	config         *Config
	clusterManager *citadel.ClusterManager

	logger = log.New(os.Stderr, "[bastion] ", log.LstdFlags)
)

func init() {
	flag.StringVar(&configPath, "conf", "", "config file")
	flag.Parse()
}

func destroy(w http.ResponseWriter, r *http.Request) {
	var container *citadel.Container
	if err := json.NewDecoder(r.Body).Decode(&container); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := clusterManager.RemoveContainer(container); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func run(w http.ResponseWriter, r *http.Request) {
	var container *citadel.Container
	if err := json.NewDecoder(r.Body).Decode(&container); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	count := 1
	qCount := r.FormValue("count")
	if qCount != "" {
		v, err := strconv.Atoi(qCount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		count = v
	}
	logger.Printf("count: %s\n", count)

	var transactions []*citadel.Transaction
	for i := 0; i < count; i++ {
		cc := *container
		t, err := clusterManager.ScheduleContainer(&cc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		transactions = append(transactions, t)
	}

	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(&transactions); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func engines(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")

	if err := json.NewEncoder(w).Encode(config.Engines); err != nil {
		logger.Println(err)
	}
}

func containers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")

	containers, err := clusterManager.ListContainers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(containers); err != nil {
		logger.Println(err)
	}
}

func main() {
	if err := loadConfig(); err != nil {
		logger.Fatal(err)
	}

	tlsConfig, err := getTLSConfig()
	if err != nil {
		logger.Fatal(err)
	}

	for _, d := range config.Engines {
		if err := setEngineClient(d, tlsConfig); err != nil {
			logger.Fatal(err)
		}
	}

	clusterManager = citadel.NewClusterManager(config.Engines, logger)

	go metrics.Log(metrics.DefaultRegistry, 10*time.Second, logger)

	labelScheduler := &citadel.LabelScheduler{}

	uniqueScheduler := &citadel.UniqueScheduler{}

	multiScheduler := citadel.NewMultiScheduler(
		&citadel.LabelScheduler{},
		&citadel.UniqueScheduler{},
	)

	clusterManager.RegisterScheduler("service", labelScheduler)
	clusterManager.RegisterScheduler("unique", uniqueScheduler)
	clusterManager.RegisterScheduler("multi", multiScheduler)

	r := mux.NewRouter()
	r.HandleFunc("/containers", containers).Methods("GET")
	r.HandleFunc("/run", run).Methods("POST")
	r.HandleFunc("/destroy", destroy).Methods("POST")

	logger.Printf("bastion listening on %s\n", config.ListenAddr)
	if err := http.ListenAndServe(config.ListenAddr, r); err != nil {
		logger.Fatal(err)
	}
}
