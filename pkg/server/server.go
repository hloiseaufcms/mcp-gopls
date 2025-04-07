package server

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/hloiseaufcms/MCPLSPGO/pkg/lsp/client"
	"github.com/hloiseaufcms/MCPLSPGO/pkg/tools"
)

type Service struct {
	server      *server.MCPServer
	lspClient   client.LSPClient
	logFile     *os.File
	clientMutex sync.Mutex
}

func NewService() (*Service, error) {
	logFile, err := os.OpenFile("/tmp/mcplspgo.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.SetOutput(os.Stdout)
		log.Printf("Unable to open log file: %v, using stderr", err)
	} else {
		log.SetOutput(logFile)
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	}

	log.Println("Starting MCP LSP Go service...")

	svc := &Service{
		logFile: logFile,
	}

	if err := svc.initLSPClient(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return nil, fmt.Errorf("failed to initialize LSP client: %w", err)
	}

	svc.server = server.NewMCPServer(
		"MCP LSP Go",
		"1.0.0",
	)

	return svc, nil
}

func (s *Service) initLSPClient() error {
	s.clientMutex.Lock()
	defer s.clientMutex.Unlock()

	if s.lspClient != nil {
		log.Println("Closing existing LSP client before reinitializing...")
		s.lspClient.Close()
		s.lspClient = nil
	}

	lspClient, err := client.NewGoplsClient()
	if err != nil {
		return fmt.Errorf("failed to create LSP client: %w", err)
	}

	log.Println("LSP client created, initializing...")

	var initErr error
	for retries := range 3 {
		initErr = lspClient.Initialize()
		if initErr == nil {
			break
		}
		log.Printf("Failed to initialize LSP client (attempt %d/3): %v", retries+1, initErr)
		time.Sleep(500 * time.Millisecond)
	}

	if initErr != nil {
		lspClient.Close()
		return fmt.Errorf("failed to initialize LSP client after multiple attempts: %w", initErr)
	}

	log.Println("LSP client successfully initialized")
	s.lspClient = lspClient
	return nil
}

func (s *Service) resetLSPClientIfNeeded(err error) bool {
	if err != nil && strings.Contains(err.Error(), "client closed") {
		log.Printf("Detected closed client, attempting to reinitialize: %v", err)
		initErr := s.initLSPClient()
		if initErr != nil {
			log.Printf("Failed to reinitialize LSP client: %v", initErr)
			return false
		}
		log.Printf("LSP client successfully reinitialized")
		return true
	}
	return false
}

func (s *Service) GetLSPClient() client.LSPClient {
	s.clientMutex.Lock()
	defer s.clientMutex.Unlock()
	return s.lspClient
}

func (s *Service) RegisterTools() {
	log.Println("Registering LSP tools...")
	lspTools := tools.NewLSPTools(s.lspClient)
	log.Println("LSP tools created")
	lspTools.SetClientGetter(func() client.LSPClient {
		return s.GetLSPClient()
	})
	log.Println("LSP client retrieved")
	lspTools.SetResetFunc(func(err error) bool {
		return s.resetLSPClientIfNeeded(err)
	})
	log.Println("LSP client reset configured")
	lspTools.Register(s.server)
	log.Println("LSP tools registered")
}

func (s *Service) Start() error {
	log.Println("Starting MCP server...")
	err := server.ServeStdio(s.server)
	log.Println("MCP server started successfully")
	return err
}
