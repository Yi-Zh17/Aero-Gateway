package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const (
	backend_url    = "http://localhost:9090"
	listen_on_port = ":8080"
	wasm_path      = "../plugin/filter/target/wasm32-unknown-unknown/release/filter.wasm"
	block_header   = "Block"
)

func middleware(ctx context.Context, mod api.Module,
	allocate api.Function, check_header api.Function, free_memory api.Function, proxy *httputil.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get header to be injected
		header_json, err := json.Marshal(r.Header)
		if err != nil {
			log.Fatal(err)
		}

		// Call allocate
		ptr, err := allocate.Call(ctx, uint64(len(header_json)))
		if err != nil {
			log.Fatal(err)
		}

		// Write to memory
		write := mod.Memory().Write(uint32(ptr[0]), header_json)
		if !write {
			log.Fatal("Write failed")
		}

		// Call check_header
		res, err := check_header.Call(ctx, ptr[0], uint64(len(header_json)))
		if err != nil {
			log.Fatal(err)
		}

		defer func() {
			_, err := free_memory.Call(ctx, ptr[0], uint64(len(header_json)), uint64(len(header_json)))
			if err != nil {
				log.Fatal(err)
			}
		}()

		if res[0] == 1 {
			http.Error(w, "Forbidden", http.StatusForbidden)
			println("Request blocked")
			return
		} else {
			println("Request passed")
			proxy.ServeHTTP(w, r)
		}
	}
}

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

	// Instantiate wasm
	mod, err := r.Instantiate(ctx, file)
	if err != nil {
		log.Fatal(err)
	}

	// Load allocation
	allocate := mod.ExportedFunction("allocate_memory")
	if allocate == nil {
		log.Fatal("Function allocate not found")
	}

	// Load check header
	check_header := mod.ExportedFunction("check_header")
	if check_header == nil {
		log.Fatal("Function check_header not found")
	}

	// Load free memory
	free_memory := mod.ExportedFunction("free_memory")
	if free_memory == nil {
		log.Fatal("Function free_memory not found")
	}

	// Instantiate a reverse proxy engine
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(u)
			r.Out.Header.Set("Aero-proxy", "active")
			r.Out.Host = r.In.Host
		},
	}

	// Create a middleware
	wrapped_handler := middleware(ctx, mod, allocate, check_header, free_memory, proxy)

	// Listen on port
	http.ListenAndServe(listen_on_port, wrapped_handler)
}
