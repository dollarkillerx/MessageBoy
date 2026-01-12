package main

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"sync"
)

type ForwardConfig struct {
	Local  string `json:"local"`
	Export string `json:"export"`
}

func main() {
	configFile := "config.json"
	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			createDefaultConfig(configFile)
			log.Printf("配置文件不存在，已创建示例配置: %s", configFile)
			log.Println("请修改配置后重新运行")
			os.Exit(0)
		}
		log.Fatalf("读取配置文件失败: %v", err)
	}

	var configs []ForwardConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		log.Fatalf("解析配置文件失败: %v", err)
	}

	if len(configs) == 0 {
		log.Fatal("配置文件为空")
	}

	var wg sync.WaitGroup
	for _, cfg := range configs {
		wg.Add(1)
		go func(c ForwardConfig) {
			defer wg.Done()
			startForward(c)
		}(cfg)
	}
	wg.Wait()
}

func createDefaultConfig(path string) {
	defaultCfg := []ForwardConfig{
		{Local: "0.0.0.0:8080", Export: "127.0.0.1:80"},
		{Local: "0.0.0.0:8443", Export: "127.0.0.1:443"},
	}
	data, _ := json.MarshalIndent(defaultCfg, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Fatalf("创建配置文件失败: %v", err)
	}
}

func startForward(cfg ForwardConfig) {
	listener, err := net.Listen("tcp", cfg.Local)
	if err != nil {
		log.Printf("监听 %s 失败: %v", cfg.Local, err)
		return
	}
	defer listener.Close()

	log.Printf("TCP转发启动: %s -> %s", cfg.Local, cfg.Export)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("接受连接失败: %v", err)
			continue
		}
		go handleConnection(conn, cfg.Export)
	}
}

func handleConnection(src net.Conn, target string) {
	defer src.Close()

	dst, err := net.Dial("tcp", target)
	if err != nil {
		log.Printf("连接目标 %s 失败: %v", target, err)
		return
	}
	defer dst.Close()

	log.Printf("新连接: %s -> %s -> %s", src.RemoteAddr(), src.LocalAddr(), target)

	var wg sync.WaitGroup
	wg.Add(2)

	// src -> dst
	go func() {
		defer wg.Done()
		io.Copy(dst, src)
		dst.(*net.TCPConn).CloseWrite()
	}()

	// dst -> src
	go func() {
		defer wg.Done()
		io.Copy(src, dst)
		src.(*net.TCPConn).CloseWrite()
	}()

	wg.Wait()
}
