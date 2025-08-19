package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chzyer/readline"
)

// Message representa uma mensagem no histórico da conversa.
type Message struct {
	Role    string
	Content string
}

// AIClient define a interface para clientes de IA (Gemini, Grok, OpenAI).
type AIClient interface {
	SendMessage(input string) (string, error)
	AddMessage(role, content string)
	GetHistory() []Message
}

// Config contém a configuração, incluindo o modelo selecionado, chaves de API e opções automáticas.
type Config struct {
	SelectedModel string            `json:"selected_model"`
	APIKeys       map[string]string `json:"api_keys"`
	AutoEdit      bool              `json:"auto_edit"`
	AutoRun       bool              `json:"auto_run"`
}

// loadConfig carrega a configuração do arquivo de configuração.
func loadConfig(configFile string) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{APIKeys: make(map[string]string)}, nil
		}
		return nil, err
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	if config.APIKeys == nil {
		config.APIKeys = make(map[string]string)
	}
	// Mapeia "grok" para "grok-2-latest" por compatibilidade
	if config.SelectedModel == "grok" {
		config.SelectedModel = "grok-2-latest"
	}
	return &config, nil
}

// saveConfig salva a configuração no arquivo de configuração.
func saveConfig(configFile string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configFile), 0700); err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0600)
}

// logMessages adiciona novas mensagens do histórico ao arquivo de log.
func logMessages(logFile string, history []Message, startIdx int) error {
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := startIdx; i < len(history); i++ {
		msg := history[i]
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		logEntry := fmt.Sprintf("[%s] %s: %s\n", timestamp, msg.Role, msg.Content)
		if _, err := f.WriteString(logEntry); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "arisu")
	configFile := filepath.Join(configDir, "config.json")

	// Configura o diretório e arquivo de log
	logDir := filepath.Join(configDir, "log")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		fmt.Printf("Erro ao criar diretório de log: %v\n", err)
		return
	}
	timestamp := time.Now().Format("20060102_150405")
	logFile := filepath.Join(logDir, "conversation_"+timestamp+".log")

	config, err := loadConfig(configFile)
	if err != nil {
		fmt.Printf("Erro ao carregar config: %v\n", err)
		return
	}

	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "--setmodel":
			if len(args) < 2 {
				fmt.Println("Uso: arisu --setmodel <model>")
				return
			}
			model := args[1]
			if model == "grok" {
				model = "grok-2-latest"
			}
			config.SelectedModel = model
			if err := saveConfig(configFile, config); err != nil {
				fmt.Printf("Erro ao salvar config: %v\n", err)
				return
			}
			fmt.Printf("Modelo selecionado definido como %s\n", model)
			return
		case "--auto-edit":
			if len(args) < 2 || (args[1] != "true" && args[1] != "false") {
				fmt.Println("Uso: arisu --auto-edit true/false")
				return
			}
			config.AutoEdit = args[1] == "true"
			if err := saveConfig(configFile, config); err != nil {
				fmt.Printf("Erro ao salvar config: %v\n", err)
				return
			}
			fmt.Printf("Auto-edit definido como %v\n", config.AutoEdit)
			return
		case "--auto-run":
			if len(args) < 2 || (args[1] != "true" && args[1] != "false") {
				fmt.Println("Uso: arisu --auto-run true/false")
				return
			}
			config.AutoRun = args[1] == "true"
			if err := saveConfig(configFile, config); err != nil {
				fmt.Printf("Erro ao salvar config: %v\n", err)
				return
			}
			fmt.Printf("Auto-run definido como %v\n", config.AutoRun)
			return
		}
	}

	// Define Gemini como padrão se nenhum modelo for selecionado
	if config.SelectedModel == "" {
		config.SelectedModel = "gemini"
	}

	openaiModels := []string{
		"gpt-4.1-mini",
		"gpt-4.1",
		"gpt-4o",
		"gpt-4o-mini",
		"o3",
		"gpt-3.5-turbo",
	}

	geminiModels := []string{
		"gemini",
		"gemini-2.0-flash",
		"gemini-2.5-flash",
		"gemini-2.5-pro",
	}

	var provider string
	if contains(geminiModels, config.SelectedModel) {
		provider = "gemini"
	} else if strings.HasPrefix(config.SelectedModel, "grok-") {
		provider = "grok"
	} else if contains(openaiModels, config.SelectedModel) {
		provider = "openai"
	} else {
		fmt.Println("Modelo selecionado inválido na configuração.")
		return
	}

	apiKey, ok := config.APIKeys[provider]
	if !ok || apiKey == "" {
		fmt.Printf("Digite sua chave API do %s: ", provider)
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			apiKey = scanner.Text()
		}
		if apiKey == "" {
			fmt.Println("Erro: Nenhuma chave API fornecida.")
			return
		}
		config.APIKeys[provider] = apiKey
		if err := saveConfig(configFile, config); err != nil {
			fmt.Printf("Erro ao salvar config: %v\n", err)
		}
	}

	var client AIClient
	if provider == "gemini" {
		model := config.SelectedModel
		if model == "gemini" {
			model = "gemini-2.0-flash"
		}
		client = NewClient(apiKey, model)
	} else if provider == "grok" {
		client = NewGrokClient(apiKey, config.SelectedModel)
	} else if provider == "openai" {
		client = NewOpenAIClient(apiKey, config.SelectedModel)
	}

	if len(args) > 0 {
		prompt := strings.Join(args, " ")
		response, err := client.SendMessage(prompt)
		if err != nil {
			fmt.Printf("Erro: %v\n", err)
			return
		}
		if strings.TrimSpace(response) == "" {
			fmt.Println("Erro: Nenhum resultado retornado pela API.")
		}
		handleResponse(response, client, config)
		if err := logMessages(logFile, client.GetHistory(), 0); err != nil {
			fmt.Printf("Erro ao registrar mensagens: %v\n", err)
		}
		return
	}

	fmt.Println("Para entrada de várias linhas, termine com uma linha em branco.")
	rl, err := readline.New("λ ")
	if err != nil {
		fmt.Printf("Erro ao inicializar readline: %v\n", err)
		return
	}
	defer rl.Close()

	lastLoggedIndex := 0
	for {
		var inputLines []string
		for {
			line, err := rl.Readline()
			if err == io.EOF {
				return
			}
			if err == readline.ErrInterrupt {
				continue
			}
			if err != nil {
				fmt.Printf("Erro ao ler linha: %v\n", err)
				return
			}
			line = strings.TrimSpace(line)
			if line == "exit" {
				fmt.Println("Adeus!")
				return
			}
			if line == "" {
				break
			}
			inputLines = append(inputLines, line)
		}
		if len(inputLines) == 0 {
			continue
		}
		input := strings.Join(inputLines, "\n")
		response, err := client.SendMessage(input)
		if err != nil {
			fmt.Printf("Erro: %v\n", err)
			continue
		}
		if strings.TrimSpace(response) == "" {
			fmt.Println("Erro: Nenhum resultado retornado pela API.")
		}
		handleResponse(response, client, config)
		if err := logMessages(logFile, client.GetHistory(), lastLoggedIndex); err != nil {
			fmt.Printf("Erro ao registrar mensagens: %v\n", err)
		}
		lastLoggedIndex = len(client.GetHistory())
	}
}

// contains verifica se uma fatia contém uma string específica.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// confirmAction solicita confirmação do usuário e retorna true se confirmado.
func confirmAction(prompt string) bool {
	fmt.Printf("%s (y/n): ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
	}
	return false
}

// Action define a interface para diferentes tipos de ações.
type Action interface {
	Execute(client AIClient, config *Config) error
}

// EditAction representa uma ação de edição.
type EditAction struct {
	Filename string
	Content  string
}

func (e EditAction) Execute(client AIClient, config *Config) error {
	if config.AutoEdit || confirmAction(fmt.Sprintf("Aplicar edição em %s?", e.Filename)) {
		if err := os.WriteFile(e.Filename, []byte(e.Content), 0644); err != nil {
			fmt.Printf("Erro ao editar %s: %v\n", e.Filename, err)
			client.AddMessage("user", fmt.Sprintf("Erro ao editar %s: %v", e.Filename, err))
			return err
		}
		fmt.Printf("Arquivo %s editado com sucesso.\n", e.Filename)
		client.AddMessage("user", fmt.Sprintf("Arquivo %s editado:\n%s", e.Filename, e.Content))
	} else {
		fmt.Printf("Edição em %s pulada.\n", e.Filename)
	}
	return nil
}

// RunAction representa uma ação de execução de comando.
type RunAction struct {
	Command string
}

func (r RunAction) Execute(client AIClient, config *Config) error {
	if config.AutoRun || confirmAction(fmt.Sprintf("Executar comando: %s?", r.Command)) {
		var outputBuf bytes.Buffer
		cmd := exec.Command("bash", "-c", r.Command)
		cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
		cmd.Stderr = io.MultiWriter(os.Stderr, &outputBuf)
		err := cmd.Run()
		if err != nil {
			fmt.Printf("Comando falhou com erro: %v\n", err)
			client.AddMessage("user", fmt.Sprintf("Comando falhou: %s\nErro: %v", r.Command, err))
			return err
		}
		output := outputBuf.String()
		if output != "" {
			client.AddMessage("user", "Saída do comando:\n"+output)
		}
	} else {
		fmt.Printf("Comando pulado: %s\n", r.Command)
	}
	return nil
}

// ReadAction representa uma ação de leitura.
type ReadAction struct {
	Filename string
}

func (r ReadAction) Execute(client AIClient, config *Config) error {
	content, err := os.ReadFile(r.Filename)
	if err != nil {
		fmt.Printf("Erro ao ler %s: %v\n", r.Filename, err)
		client.AddMessage("user", fmt.Sprintf("Erro ao ler %s: %v", r.Filename, err))
		return err
	}
	fmt.Printf("Conteúdo de %s:\n%s\n", r.Filename, string(content))
	client.AddMessage("user", fmt.Sprintf("Conteúdo de %s:\n%s", r.Filename, string(content)))
	return nil
}

// extractActions extrai ações da resposta na ordem em que aparecem.
func extractActions(response string) []Action {
	var actions []Action
	start := 0
	for {
		// Encontra a próxima tag
		editIdx := strings.Index(response[start:], "<EDIT>")
		runIdx := strings.Index(response[start:], "<RUN>")
		readIdx := strings.Index(response[start:], "<READ>")

		// Determina a tag mais próxima
		indices := []int{editIdx, runIdx, readIdx}
		minIdx := -1
		tagType := ""
		for i, idx := range indices {
			if idx != -1 && (minIdx == -1 || idx < minIdx) {
				minIdx = idx
				switch i {
				case 0:
					tagType = "EDIT"
				case 1:
					tagType = "RUN"
				case 2:
					tagType = "READ"
				}
			}
		}
		if minIdx == -1 {
			break
		}
		minIdx += start

		// Encontra a tag de fechamento
		endTag := "</" + tagType + ">"
		endIdx := strings.Index(response[minIdx:], endTag)
		if endIdx == -1 {
			break
		}
		endIdx += minIdx + len(endTag)

		// Extrai o conteúdo entre as tags
		contentStart := minIdx + len("<"+tagType+">")
		content := strings.TrimSpace(response[contentStart : endIdx-len(endTag)])

		switch tagType {
		case "EDIT":
			lines := strings.SplitN(content, "\n", 2)
			if len(lines) == 2 {
				filename := strings.TrimSpace(lines[0])
				editContent := strings.TrimSpace(lines[1])
				actions = append(actions, EditAction{Filename: filename, Content: editContent})
			}
		case "RUN":
			actions = append(actions, RunAction{Command: content})
		case "READ":
			actions = append(actions, ReadAction{Filename: content})
		}

		start = endIdx
	}
	return actions
}

// handleResponse processa a resposta do AI, executando ações na ordem em que aparecem.
func handleResponse(response string, client AIClient, config *Config) {
	actions := extractActions(response)
	for _, action := range actions {
		if err := action.Execute(client, config); err != nil {
			fmt.Printf("Erro ao executar ação: %v\n", err)
		}
	}
}
