package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync/atomic"

	"github.com/tetratelabs/wazero"
)

const (
	backend_url    = "http://localhost:9090"
	listen_on_port = ":8080"
	admin_port     = ":8081"
	wasm_path      = "../plugin/filter/target/wasm32-unknown-unknown/release/filter.wasm"
	num_channel    = 32
)

func main() {
	// Parse upstream server url
	u, err := url.Parse(backend_url)
	if err != nil {
		log.Fatal(err)
	}

	// Read wasm
	file, err := os.ReadFile(wasm_path)
	if err != nil {
		log.Fatal(err)
	}

	// Initiate context
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	// Compile wasm module
	compiledMod, err := r.CompileModule(ctx, file)
	if err != nil {
		log.Fatal(err)
	}

	// Create a buffered channel
	channel := make(chan Instance, num_channel)

	// Instantiate modules and put them into channel
	for i := 0; i < num_channel; i++ {
		// Instantiate a module
		mod, err := r.InstantiateModule(ctx, compiledMod, wazero.NewModuleConfig().WithName("")) // Generate a random name for the instance
		if err != nil {
			log.Fatal(err)
		}

		// Load allocation
		allocate := mod.ExportedFunction("allocate_memory")
		if allocate == nil {
			log.Fatal("Function allocate not found")
		}

		// Load check header
		process_request := mod.ExportedFunction("process_request")
		if process_request == nil {
			log.Fatal("Function process_request not found")
		}

		// Load free memory
		free_memory := mod.ExportedFunction("free_memory")
		if free_memory == nil {
			log.Fatal("Function free_memory not found")
		}

		// Create an instance struct
		instance := Instance{mod, allocate, process_request, free_memory}

		// Put into channel
		channel <- instance
	}

	// Wrap channel in a pointer
	var pool atomic.Pointer[chan Instance]
	pool.Store(&channel)

	// Instantiate a reverse proxy engine
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(u)
			r.Out.Header.Set("Aero-proxy", "active")
			r.Out.Host = r.In.Host
		},
	}

	// Build metrics
	metrics := &Metrics{}

	// Set up admin handler
	adminMux := http.NewServeMux()
	adminMux.Handle("/admin/upload", withCORS(uploadHandler(ctx, r, &pool)))
	adminMux.Handle("/admin/metrics", withCORS(metricsHandler(metrics)))

	go http.ListenAndServe(admin_port, adminMux)

	// Create a middleware
	wrapped_handler := middleware(ctx, &pool, metrics, proxy)

	// Listen on port
	http.ListenAndServe(listen_on_port, wrapped_handler)
}
