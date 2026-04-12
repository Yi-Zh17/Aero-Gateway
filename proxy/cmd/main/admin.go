package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync/atomic"

	"github.com/tetratelabs/wazero"
)

type Metrics struct {
	TotalRequests   atomic.Uint64
	BlockedRequests atomic.Uint64
	LastExecutionNs atomic.Uint64
}

func metricsHandler(m *Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		total := m.TotalRequests.Load()
		blocked := m.BlockedRequests.Load()
		lastExec := m.LastExecutionNs.Load()

		field := struct {
			TotalRequests   uint64 `json:"total_requests"`
			BlockedRequests uint64 `json:"blocked_requests"`
			LastExecutionNs uint64 `json:"last_execution_ns"`
		}{
			total,
			blocked,
			lastExec,
		}

		enc := json.NewEncoder(w)
		w.Header().Set("Content-Type", "application/json")
		enc.Encode(field)
	}
}

func withCORS(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.Header().Add("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Add("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

func uploadHandler(ctx context.Context, runtime wazero.Runtime, pool *atomic.Pointer[chan Instance]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		file, _, err := r.FormFile("plugin")
		if err != nil {
			log.Printf("Unable to get plugin file")
			http.Error(w, "Cannot get plugin file", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			log.Printf("Cannot read the plugin file")
			http.Error(w, "Cannot read the plugin file", http.StatusInternalServerError)
			return
		}

		if len(content) == 0 {
			log.Printf("Error: empty file")
			http.Error(w, "Empty file", http.StatusBadRequest)
			return
		}

		// Hot swap starts here
		// Compile module
		compiledMod, err := runtime.CompileModule(ctx, content)
		if err != nil {
			log.Printf("Error: cannot compile the module")
			http.Error(w, "Cannot compile module", http.StatusInternalServerError)
			return
		}

		// Instantiate channel
		channel := make(chan Instance, num_channel)

		// Instantiate modules and put them into channel
		for i := 0; i < num_channel; i++ {
			// Instantiate a module
			mod, err := runtime.InstantiateModule(ctx, compiledMod, wazero.NewModuleConfig().WithName("")) // Generate a random name for the instance
			if err != nil {
				log.Printf("Error: %s", err)
				releaseInstance(ctx, channel)
				http.Error(w, "Fail to instantiate a module", http.StatusInternalServerError)
				return
			}

			// Load allocation
			allocate := mod.ExportedFunction("allocate_memory")
			if allocate == nil {
				log.Printf("Failed to load function allocate_memory")
				releaseInstance(ctx, channel)
				http.Error(w, "Failed to load function allocate_memory", http.StatusInternalServerError)
				return
			}

			// Load check header
			process_request := mod.ExportedFunction("process_request")
			if process_request == nil {
				log.Printf("Failed to load function process_request")
				releaseInstance(ctx, channel)
				http.Error(w, "Failed to load function process_request", http.StatusInternalServerError)
				return
			}

			// Load free memory
			free_memory := mod.ExportedFunction("free_memory")
			if free_memory == nil {
				log.Printf("Failed to load function free_memory")
				releaseInstance(ctx, channel)
				http.Error(w, "Failed to load function free_memory", http.StatusInternalServerError)
				return
			}

			// Create an instance struct
			instance := Instance{mod, allocate, process_request, free_memory}

			// Put into channel
			channel <- instance
		}

		// Swap channel
		pool.Swap(&channel)

		// Respond success
		w.WriteHeader(http.StatusOK)
	}
}

func releaseInstance(ctx context.Context, channel chan Instance) {
	// Release any created instance
	for len(channel) > 0 {
		inst := <-channel
		inst.mod.Close(ctx)
	}

}
