package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
	configPath := flag.String(
		"config",
		"configs/flexiproxy.yaml",
		"Path to configuration file",
	)

	showVersion := flag.Bool(
		"version",
		false,
		"Print version information and exit",
	)

	flag.Parse()

	if *showVersion {
		fmt.Printf("FlexiProxy v%s (built: %s)\n", version, buildTime)
		os.Exit(0)
	}

	// ------------------------------------------------------------
	// LOAD CONFIGURATION
	// ------------------------------------------------------------

	initialCfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		log.Fatalf("Fatal: Configuration error: %v", err)
	}

	configMgr := config.NewManager(initialCfg, *configPath)

	log.Printf("FlexiProxy v%s initializing...", version)
	log.Printf("[Config] Loaded successfully from: %s", *configPath)

	// ------------------------------------------------------------
	// CREATE PROXY ENGINE
	// ------------------------------------------------------------

	proxyEngine, err := proxy.NewProxyEngine(configMgr)
	if err != nil {
		log.Fatalf("Fatal: Failed to create proxy engine: %v", err)
	}
	defer proxyEngine.Close()

	// ------------------------------------------------------------
	// ADMIN API + DASHBOARD
	// ------------------------------------------------------------

	adminServer := api.NewAdminServer(
		configMgr,
		proxyEngine.Backends(),
	)

	// ------------------------------------------------------------
	// DETECT RENDER
	// ------------------------------------------------------------

	renderMode := false
	proxyPort := initialCfg.Server.Port

	if portValue := os.Getenv("PORT"); portValue != "" {
		port, err := strconv.Atoi(portValue)

		if err != nil || port <= 0 || port > 65535 {
			log.Fatalf("Fatal: Invalid PORT environment variable: %q", portValue)
		}

		renderMode = true
		proxyPort = port

		log.Printf("[Cloud] PORT detected: %d", proxyPort)
	}

	// ------------------------------------------------------------
	// PUBLIC HANDLER
	// ------------------------------------------------------------

	var publicHandler http.Handler = proxyEngine

	if renderMode {
		// Render exposes one public HTTP port.
		// Therefore the Admin API + Dashboard and the reverse
		// proxy are served through the same public port.

		mux := http.NewServeMux()

		// Admin API
		mux.Handle("/api/", adminServer)

		// Metrics endpoint
		mux.Handle("/metrics", adminServer)

		// Dashboard
		mux.Handle("/dashboard/", adminServer)

		// Everything else goes to the reverse proxy.
		mux.Handle("/", proxyEngine)

		publicHandler = mux

		log.Println("[Cloud] Admin API + Dashboard + Proxy sharing public port")
	}

	// ------------------------------------------------------------
	// PUBLIC HTTP SERVER
	// ------------------------------------------------------------

	proxyAddr := fmt.Sprintf("0.0.0.0:%d", proxyPort)

	proxyServer := &http.Server{
		Addr:    proxyAddr,
		Handler: publicHandler,

		ReadTimeout:  initialCfg.Server.ReadTimeout,
		WriteTimeout: initialCfg.Server.WriteTimeout,
		IdleTimeout:  initialCfg.Server.IdleTimeout,
	}

	// ------------------------------------------------------------
	// LOCAL ADMIN SERVER
	// ------------------------------------------------------------

	var adminHTTPServer *http.Server

	if !renderMode {
		adminAddr := fmt.Sprintf(":%d", initialCfg.Server.AdminPort)

		adminHTTPServer = &http.Server{
			Addr:    adminAddr,
			Handler: adminServer,

			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		go func() {
			log.Printf(
				"FlexiProxy Admin API & Dashboard listening on http://localhost%s/dashboard/",
				adminAddr,
			)

			if err := adminHTTPServer.ListenAndServe(); err != nil &&
				err != http.ErrServerClosed {

				log.Fatalf(
					"Fatal: Admin HTTP listener error: %v",
					err,
				)
			}
		}()
	}

	// ------------------------------------------------------------
	// HOT RELOAD
	// ------------------------------------------------------------

	reloadSig := make(chan os.Signal, 1)

	signal.Notify(
		reloadSig,
		syscall.SIGHUP,
	)

	go func() {
		for range reloadSig {

			log.Println(
				"[HotReload] SIGHUP signal received. Reloading configuration from disk...",
			)

			if _, err := configMgr.ReloadFromFile(*configPath); err != nil {

				log.Printf(
					"[HotReload] Error reloading config: %v",
					err,
				)

			} else {

				log.Println(
					"[HotReload] Configuration reloaded successfully without downtime.",
				)
			}
		}
	}()

	// ------------------------------------------------------------
	// START PUBLIC SERVER
	// ------------------------------------------------------------

	go func() {

		if initialCfg.Server.TLS.Enabled && !renderMode {

			log.Printf(
				"FlexiProxy HTTPS Reverse Proxy listening on https://localhost:%d",
				proxyPort,
			)

			if err := proxyServer.ListenAndServeTLS(
				initialCfg.Server.TLS.CertFile,
				initialCfg.Server.TLS.KeyFile,
			); err != nil && err != http.ErrServerClosed {

				log.Fatalf(
					"Fatal: HTTPS proxy listener error: %v",
					err,
				)
			}

		} else {

			if renderMode {

				log.Printf(
					"FlexiProxy Cloud Server listening on 0.0.0.0:%d",
					proxyPort,
				)

			} else {

				log.Printf(
					"FlexiProxy HTTP Reverse Proxy listening on http://localhost:%d",
					proxyPort,
				)
			}

			if err := proxyServer.ListenAndServe(); err != nil &&
				err != http.ErrServerClosed {

				log.Fatalf(
					"Fatal: HTTP proxy listener error: %v",
					err,
				)
			}
		}
	}()

	// ------------------------------------------------------------
	// GRACEFUL SHUTDOWN
	// ------------------------------------------------------------

	stop := make(chan os.Signal, 1)

	signal.Notify(
		stop,
		os.Interrupt,
		syscall.SIGTERM,
	)

	<-stop

	log.Println(
		"Received termination signal. Initiating graceful shutdown...",
	)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)

	defer cancel()

	_ = proxyServer.Shutdown(ctx)

	if adminHTTPServer != nil {
		_ = adminHTTPServer.Shutdown(ctx)
	}

	log.Println(
		"FlexiProxy proxy engine and admin services stopped cleanly.",
	)
}