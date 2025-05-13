package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"natter/internal/config"
	ilog "natter/internal/log"
	"natter/internal/orchestrator"

	"go.uber.org/zap"
)

func usage() {
	prog := os.Args[0]
	fmt.Fprintf(os.Stderr, "Usage:\n  %s [options] [host] <port>\n", prog)
	fmt.Fprintf(os.Stderr, "Options:\n  -c string   Path to JSON config file\n  -v          Enable debug logging\n  -t          Enable HTTP test server (port mode only)\n")
	fmt.Fprintf(os.Stderr, "Examples:\n  %s 2888\n  %s 127.0.0.1 2888\n  %s -c config.json\n  %s -t 2888\n", prog, prog, prog, prog)
}

func main() {
	// 解析命令行参数
	configPath := flag.String("c", "", "Path to JSON config file")
	verbose := flag.Bool("v", false, "Enable debug logging")
	testHTTP := flag.Bool("t", false, "Enable HTTP test server (port mode only)")
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()

	// 构造配置
	var cfg *config.Config
	var host string
	var port int
	var err error
	if *configPath != "" {
		// 使用配置文件模式
		cfg, err = config.Load(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
			os.Exit(1)
		}
	} else {
		// 端口模式
		if len(args) != 1 && len(args) != 2 {
			usage()
			os.Exit(1)
		}
		host = "0.0.0.0"
		portArg := args[0]
		if len(args) == 2 {
			host = args[0]
			portArg = args[1]
		}
		port, err = strconv.Atoi(portArg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid port: %v\n", err)
			os.Exit(1)
		}

		// 临时配置
		cfg = &config.Config{
			StunServer:   config.StunServer{TCP: nil, UDP: nil},
			KeepAlive:    "www.qq.com",
			Interval:     10,
			OpenPort:     config.OpenPort{TCP: []string{fmt.Sprintf("%s:%d", host, port)}},
			ForwardPort:  config.ForwardPort{},
			StatusReport: config.StatusReport{StatusFile: "status.json"},
			Logging:      config.Logging{},
		}

		// 如果启用 HTTP 测试服务器
		if *testHTTP {
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, "<h1>It works!</h1><hr/>Natter")
			})
			addr := fmt.Sprintf("%s:%d", host, port)
			fmt.Printf("[INFO] HTTP test server listening on %s\n", addr)
			go func() {
				if err := http.ListenAndServe(addr, mux); err != nil {
					fmt.Fprintf(os.Stderr, "HTTP test server error: %v\n", err)
				}
			}()
		}
	}

	// 初始化日志
	level := "info"
	if *verbose {
		level = "debug"
	}
	logger, err := ilog.New(level, cfg.Logging.LogFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}

	// 创建 orchestrator
	n, err := orchestrator.New(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to create Natter", zap.Error(err))
	}

	// 捕捉中断信号，优雅退出
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("Starting natter")
	n.Run(ctx)
	logger.Info("Exited natter")
}
