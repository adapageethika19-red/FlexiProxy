package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"flexiproxy/internal/api"
	"flexiproxy/internal/config"
	"flexiproxy/internal/proxy"
)

var (
	version   = "0.1.0"
	buildTime = "unknown"
)

func main() {
	configPath := flag.String("config", "configs/flexiproxy.yaml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("FlexiProxy v%s (built: %s)\n", version, buildTime)
		os.Exit(0)
	}

	initialCfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		log.Fatalf("Fatal: Configuration error: %v", err)
	}

	configMgr := config.NewManager(initialCfg, *configPath)

	log.Printf("FlexiProxy v%s initializing...", version)
	log.Printf("[Config] Loaded successfully from: %s", *configPath)

	proxyEngine, err := proxy.NewProxyEngine(configMgr)
	if err != nil {
		log.Fatalf("Fatal: Failed to create proxy engine: %v", err)
	}
	defer proxyEngine.Close()

	proxyAddr := fmt.Sprintf(":%d", initialCfg.Server.Port)
	proxyServer := &http.Server{
		Addr:         proxyAddr,
		Handler:      proxyEngine,
		ReadTimeout:  initialCfg.Server.ReadTimeout,
		WriteTimeout: initialCfg.Server.WriteTimeout,
		IdleTimeout:  initialCfg.Server.IdleTimeout,
	}

	// Launch Admin REST API & Dashboard Server
	adminServer := api.NewAdminServer(configMgr, proxyEngine.Backends())
	adminAddr := fmt.Sprintf(":%d", initialCfg.Server.AdminPort)
	adminHTTPServer := &http.Server{
		Addr:         adminAddr,
		Handler:      adminServer,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Trap SIGINT / SIGTERM for graceful shutdown, SIGHUP for hot reload
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	reloadSig := make(chan os.Signal, 1)
	signal.Notify(reloadSig, syscall.SIGHUP)

	go func() {
		for range reloadSig {
			log.Println("[HotReload] SIGHUP signal received. Reloading configuration from disk...")
			if _, err := configMgr.ReloadFromFile(*configPath); err != nil {
				log.Printf("[HotReload] Error reloading config: %v", err)
			} else {
				log.Println("[HotReload] Configuration reloaded successfully without downtime.")
			}
		}
	}()

	go func() {
		if initialCfg.Server.TLS.Enabled {
			log.Printf("FlexiProxy HTTPS Reverse Proxy listening on https://localhost%s", proxyAddr)
			if err := proxyServer.ListenAndServeTLS(initialCfg.Server.TLS.CertFile, initialCfg.Server.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Fatal: HTTPS proxy listener error: %v", err)
			}
		} else {
			log.Printf("FlexiProxy HTTP Reverse Proxy listening on http://localhost%s", proxyAddr)
			if err := proxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Fatal: HTTP proxy listener error: %v", err)
			}
		}
	}()

	go func() {
		log.Printf("FlexiProxy Admin API & Dashboard listening on http://localhost%s/dashboard/", adminAddr)
		if err := adminHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Fatal: Admin HTTP listener error: %v", err)
		}
	}()

	<-stop
	log.Println("Received termination signal. Initiating graceful shutdown...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = adminHTTPServer.Shutdown(ctx)
	_ = proxyServer.Shutdown(ctx)

	log.Println("FlexiProxy proxy engine and admin services stopped cleanly.")
}
